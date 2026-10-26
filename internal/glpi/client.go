package glpi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
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
	AssignedUser   string `json:"assigned_user"`
	PriorityID     int    `json:"priority_id"`
	Priority       string `json:"priority"`
	UrgencyID      int    `json:"urgency_id"`
	Category       string `json:"category"`
	CategoryFull   string `json:"category_full"`
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
	userCache  map[string]string
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
		userCache: map[string]string{
			"20":  "Roberto Casali",
			"11":  "none",
			"18":  "Delcio Matos",
			"10":  "Wesley Benedito",
			"19":  "Augusto Correia",
			"16":  "Victor Menez",
			"166": "Arthur Salles",
			"9":   "Luan Costa",
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

	// 2. Montar Critérios de Busca (Apenas status não solucionado/não antigo: field 12 = notold)
	q := url.Values{}
	q.Set("criteria[0][field]", "12")
	q.Set("criteria[0][searchtype]", "equals")
	q.Set("criteria[0][value]", "notold")

	q.Set("sort", "2") // ID do Chamado
	q.Set("order", "DESC")
	if limit <= 0 {
		limit = 100
	}
	q.Set("range", fmt.Sprintf("0-%d", limit))

	// forcedisplay (adicionado field 5: Técnico atribuído)
	forcedFields := []string{"1", "2", "3", "4", "5", "7", "8", "10", "12", "15", "19", "22", "71"}
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

	// 1. Popula cache dinâmico de IDs de usuários a partir dos autores e emails retornados
	for _, item := range rawResult.Data {
		uID := parseString(item["4"])
		uMail := parseString(item["22"])
		if uID != "" && uMail != "" {
			if _, exists := c.userCache[uID]; !exists {
				c.userCache[uID] = formatDisplayName(uMail)
			}
		}
	}

	tickets := make([]Ticket, 0, len(rawResult.Data))
	for _, item := range rawResult.Data {
		idVal := parseNumber(item["2"])
		statusVal := parseNumber(item["12"])
		priorityVal := parseNumber(item["3"])
		urgencyVal := parseNumber(item["10"])

		cat := parseString(item["7"])
		reqGrp := parseString(item["71"])
		catClean := extractLastCategory(cat)
		reqGrpClean := strings.TrimPrefix(reqGrp, "Departamentos > ")

		rawDateCreated := parseString(item["15"])
		dateCreatedBR := formatDateBR(rawDateCreated)

		reqUser := parseString(item["22"])
		if reqUser == "" {
			reqUser = parseString(item["4"])
		}
		reqUserDisplayName := formatDisplayName(reqUser)

		// Extrai o primeiro usuário atribuído (field 5), ignorando grupos
		firstTechID := parseFirstTechID(item["5"])
		assignedUserName := "-"
		if firstTechID != "" {
			if name, ok := c.userCache[firstTechID]; ok && name != "" {
				assignedUserName = name
			} else {
				fetchedName := c.fetchUserName(ctx, sessionToken, firstTechID)
				if fetchedName != "" && fetchedName != firstTechID {
					c.userCache[firstTechID] = fetchedName
					assignedUserName = fetchedName
				} else {
					assignedUserName = formatDisplayName(firstTechID)
				}
			}
		}

		ticketURL := fmt.Sprintf("%s/front/ticket.form.php?id=%d", c.cfg.GlpiURL, idVal)

		tickets = append(tickets, Ticket{
			ID:             idVal,
			Title:          parseString(item["1"]),
			StatusID:       statusVal,
			Status:         mapStatus(statusVal),
			AssignedUser:   assignedUserName,
			PriorityID:     priorityVal,
			Priority:       mapPriority(priorityVal),
			UrgencyID:      urgencyVal,
			Category:       catClean,
			CategoryFull:   cat,
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

	// Ordena única e exclusivamente por ID decrescente
	sort.SliceStable(tickets, func(i, j int) bool {
		return tickets[i].ID > tickets[j].ID
	})

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
		return decodeHTMLEntities(v)
	case float64:
		return strconv.Itoa(int(v))
	default:
		return decodeHTMLEntities(fmt.Sprintf("%v", v))
	}
}

// decodeHTMLEntities converte entidades HTML retornadas pelo GLPI (ex: "&#38;" -> "&",
// "&#62;" -> ">") em texto legível. Repete o processo algumas vezes para tratar
// valores duplamente codificados (ex: "&amp;#62;"). O frontend já faz escape na renderização.
func decodeHTMLEntities(s string) string {
	for i := 0; i < 3 && strings.Contains(s, "&"); i++ {
		decoded := html.UnescapeString(s)
		if decoded == s {
			break
		}
		s = decoded
	}
	return s
}

// extractLastCategory retorna apenas o último nível da hierarquia de categorias (ex: "Protheus > BackOffice > Fiscal > Transmissão" -> "Transmissão").
func extractLastCategory(cat string) string {
	cat = strings.TrimSpace(cat)
	if cat == "" {
		return ""
	}
	parts := strings.Split(cat, ">")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			return part
		}
	}
	return cat
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

func parseFirstTechID(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	case []interface{}:
		if len(v) > 0 {
			return parseString(v[0])
		}
	}
	return ""
}

// fetchUserName busca os dados do usuário na API do GLPI caso o ID não esteja no cache local.
func (c *Client) fetchUserName(ctx context.Context, sessionToken, userID string) string {
	if userID == "" {
		return "-"
	}
	apiURL := fmt.Sprintf("%s/apirest.php/User/%s", c.cfg.GlpiURL, userID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return userID
	}
	req.Header.Set("App-Token", c.cfg.GlpiAppToken)
	req.Header.Set("Session-Token", sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return userID
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return userID
	}

	var u struct {
		Name      string `json:"name"`
		Firstname string `json:"firstname"`
		Realname  string `json:"realname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return userID
	}

	if u.Name != "" {
		return formatDisplayName(u.Name)
	}
	fullName := strings.TrimSpace(u.Firstname + " " + u.Realname)
	if fullName != "" {
		return fullName
	}
	return userID
}
