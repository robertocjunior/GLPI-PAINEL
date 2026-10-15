package glpi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"glpi-painel/internal/config"
)

var statusMap = map[int]string{
	1: "Novo",
	2: "Atribuído",
	3: "Planejado",
	4: "Pendente",
	5: "Solucionado",
	6: "Fechado",
}

var priorityMap = map[int]string{
	1: "Muito baixa",
	2: "Baixa",
	3: "Média",
	4: "Alta",
	5: "Muito alta",
	6: "Maior",
}

type Ticket struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	StatusID       int    `json:"status_id"`
	Status         string `json:"status"`
	PriorityID     int    `json:"priority_id"`
	Priority       string `json:"priority"`
	UrgencyID      int    `json:"urgency_id"`
	Category       string `json:"category"`
	TechnicalGroup string `json:"technical_group"`
	RequesterID    string `json:"requester_id"`
	RequesterUser  string `json:"requester_user"`
	RequesterGroup string `json:"requester_group"`
	DateCreated    string `json:"date_created"`
	DateCreatedBR  string `json:"date_created_br"`
	DateMod        string `json:"date_mod"`
	URL            string `json:"url"`
}

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
	mu         sync.Mutex
}

func NewClient(cfg *config.Config) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   20 * time.Second,
		},
	}
}

// FetchTickets consulta chamados da fila do GLPI de forma otimizada
func (c *Client) FetchTickets(ctx context.Context, limit int) ([]Ticket, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	apiURL := fmt.Sprintf("%s/apirest.php", c.cfg.GlpiURL)

	// 1. Iniciar Sessão
	initReq, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/initSession", nil)
	if err != nil {
		return nil, fmt.Errorf("erro criando requisição initSession: %w", err)
	}
	initReq.Header.Set("App-Token", c.cfg.GlpiAppToken)
	initReq.Header.Set("Authorization", fmt.Sprintf("user_token %s", c.cfg.GlpiUserToken))

	resp, err := c.httpClient.Do(initReq)
	if err != nil {
		return nil, fmt.Errorf("falha ao autenticar no GLPI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GLPI initSession falhou com status %d: %s", resp.StatusCode, string(body))
	}

	var sessionResp struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return nil, fmt.Errorf("erro decodificando token de sessão GLPI: %w", err)
	}
	sessionToken := sessionResp.SessionToken

	// Garantir que a sessão é destruída no final
	defer func() {
		killReq, _ := http.NewRequestWithContext(context.Background(), "GET", apiURL+"/killSession", nil)
		if killReq != nil {
			killReq.Header.Set("App-Token", c.cfg.GlpiAppToken)
			killReq.Header.Set("Session-Token", sessionToken)
			killResp, killErr := c.httpClient.Do(killReq)
			if killErr == nil {
				_ = killResp.Body.Close()
			}
		}
	}()

	// 2. Montar Critérios de Busca (Fila de Sistemas Otimizada)
	q := url.Values{}
	q.Set("criteria[0][field]", "12")
	q.Set("criteria[0][searchtype]", "equals")
	q.Set("criteria[0][value]", "notold")

	q.Set("criteria[1][field]", "8")
	q.Set("criteria[1][searchtype]", "equals")
	q.Set("criteria[1][value]", c.cfg.TechnicalGroup)

	q.Set("criteria[2][field]", "7")
	q.Set("criteria[2][searchtype]", "notcontains")
	q.Set("criteria[2][value]", "Holmes")

	q.Set("criteria[3][field]", "7")
	q.Set("criteria[3][searchtype]", "notcontains")
	q.Set("criteria[3][value]", "Metadados")

	q.Set("criteria[4][field]", "7")
	q.Set("criteria[4][searchtype]", "notcontains")
	q.Set("criteria[4][value]", "Movimentação Funcional")

	q.Set("sort", "19") // Data de Modificação
	q.Set("order", "DESC")
	if limit <= 0 {
		limit = 100
	}
	q.Set("range", fmt.Sprintf("0-%d", limit))

	// forcedisplay
	forcedFields := []string{"1", "2", "3", "4", "7", "8", "10", "12", "15", "19", "22", "71"}
	for i, f := range forcedFields {
		q.Set(fmt.Sprintf("forcedisplay[%d]", i), f)
	}

	searchEndpoint := fmt.Sprintf("%s/search/Ticket?%s", apiURL, q.Encode())
	searchReq, err := http.NewRequestWithContext(ctx, "GET", searchEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("erro criando requisição search: %w", err)
	}
	searchReq.Header.Set("App-Token", c.cfg.GlpiAppToken)
	searchReq.Header.Set("Session-Token", sessionToken)

	searchResp, err := c.httpClient.Do(searchReq)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar chamados GLPI: %w", err)
	}
	defer searchResp.Body.Close()

	if searchResp.StatusCode != http.StatusOK && searchResp.StatusCode != 206 {
		body, _ := io.ReadAll(searchResp.Body)
		return nil, fmt.Errorf("GLPI search falhou com status %d: %s", searchResp.StatusCode, string(body))
	}

	var rawResult struct {
		TotalCount int                      `json:"totalcount"`
		Count      int                      `json:"count"`
		Data       []map[string]interface{} `json:"data"`
	}

	if err := json.NewDecoder(searchResp.Body).Decode(&rawResult); err != nil {
		return nil, fmt.Errorf("erro decodificando resposta da busca de chamados: %w", err)
	}

	tickets := make([]Ticket, 0, len(rawResult.Data))
	for _, item := range rawResult.Data {
		idVal := parseNumber(item["2"])
		statusVal := parseNumber(item["12"])
		priorityVal := parseNumber(item["3"])
		urgencyVal := parseNumber(item["10"])

		cat := parseString(item["7"])
		reqGrp := parseString(item["71"])
		catClean := strings.TrimPrefix(cat, "Protheus > ")
		reqGrpClean := strings.TrimPrefix(reqGrp, "Departamentos > ")

		rawDateCreated := parseString(item["15"])
		dateCreatedBR := formatDateBR(rawDateCreated)

		reqUser := parseString(item["22"])
		if reqUser == "" {
			reqUser = parseString(item["4"])
		}
		reqUserDisplayName := formatDisplayName(reqUser)

		ticketURL := fmt.Sprintf("%s/front/ticket.form.php?id=%d", c.cfg.GlpiURL, idVal)

		tickets = append(tickets, Ticket{
			ID:             idVal,
			Title:          parseString(item["1"]),
			StatusID:       statusVal,
			Status:         mapStatus(statusVal),
			PriorityID:     priorityVal,
			Priority:       mapPriority(priorityVal),
			UrgencyID:      urgencyVal,
			Category:       catClean,
			TechnicalGroup: parseString(item["8"]),
			RequesterID:    parseString(item["4"]),
			RequesterUser:  reqUserDisplayName,
			RequesterGroup: reqGrpClean,
			DateCreated:    rawDateCreated,
			DateCreatedBR:  dateCreatedBR,
			DateMod:        parseString(item["19"]),
			URL:            ticketURL,
		})
	}

	slog.Debug("Consulta GLPI realizada com sucesso", "tickets_found", len(tickets))
	return tickets, nil
}

func mapStatus(id int) string {
	if val, ok := statusMap[id]; ok {
		return val
	}
	return fmt.Sprintf("Status %d", id)
}

func mapPriority(id int) string {
	if val, ok := priorityMap[id]; ok {
		return val
	}
	return fmt.Sprintf("Prioridade %d", id)
}

func parseNumber(val interface{}) int {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func parseString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatDateBR(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	// Aceita formato padrão do GLPI "2006-01-02 15:04:05" ou "2006-01-02"
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.Parse(l, dateStr); err == nil {
			return t.Format("02/01/2006")
		}
	}
	// Fallback caso venha com traços
	parts := strings.Split(strings.Split(dateStr, " ")[0], "-")
	if len(parts) == 3 {
		return fmt.Sprintf("%s/%s/%s", parts[2], parts[1], parts[0])
	}
	return dateStr
}

func formatDisplayName(raw string) string {
	if raw == "" {
		return "-"
	}
	val := strings.TrimSpace(raw)
	// Remove sufixo de email caso presente (@lipetral.com.br, etc)
	if atIdx := strings.Index(val, "@"); atIdx != -1 {
		val = val[:atIdx]
	}
	// Normaliza separadores comuns (ponto, sublinhado, hífen)
	val = strings.ReplaceAll(val, ".", " ")
	val = strings.ReplaceAll(val, "_", " ")
	val = strings.ReplaceAll(val, "-", " ")

	parts := strings.Fields(val)
	if len(parts) == 0 {
		return raw
	}

	for i, p := range parts {
		r := []rune(strings.ToLower(p))
		if len(r) > 0 {
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
			parts[i] = string(r)
		}
	}
	return strings.Join(parts, " ")
}
