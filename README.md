# GLPI-PAINEL

Painel autônomo e de alta performance desenvolvido em **Go** com interface web embutida, projetado para monitoramento contínuo em TVs e displays operacionais de chamados em aberto do GLPI.

---

## 🚀 Principais Recursos

- **Backend em Go de Alto Desempenho**:
  - Consulta otimizada na API REST do GLPI (`apirest.php`), gerenciando e encerrando sessões de forma segura por requisição.
  - Ordenação rigorosa por **ID decrescente** (`DESC`), trazendo os chamados mais novos no topo.
  - Cache de resolução de nomes de técnicos e autores para exibição amigável sem chamadas extras de permissão.
- **Frontend Embutido (Zero Dependências Externas)**:
  - Todo o HTML, CSS e JavaScript são embutidos diretamente no binário via `embed.FS`.
  - Design escuro com cantos retos (0px border-radius), sem degradês, com tipografia de alta legibilidade para TVs.
- **Dois Níveis de Listagem**:
  - **Tabela Superior**: Exibe os 5 chamados mais recentes em aberto com colunas ID, Título, Categoria, Status, Atribuído a, Prioridade, Requerente e Data de Abertura.
  - **Tabela Inferior (Alertas)**: Lista persistente dedicada exclusivamente a chamados com prioridade **Alta, Muito Alta ou Crítica**, contendo ID, Título, Prioridade, Requerente e Atribuído a.
- **Sistema Inteligente de Áudio e Notificações**:
  - **Som Padrão (Aeroporto)**: Sintetizador Web Audio API de 4 notas harmônicas com sustentação ou arquivo customizado `.mp3`.
  - **Som Crítico/Emergência**: Alarme sonoro sintetizado urgente ou arquivo `.mp3` para chamados de prioridade alta/crítica.
  - **Som por Categoria**: Disparo de áudios específicos baseados na categoria do chamado recém-chegado.
  - **Destaque Visual**: Chamados recém-chegados que dispararam o alerta sonoro ficam destacados em azul sólido até a próxima atualização que traga novos chamados.

---

## ⚙️ Variáveis de Ambiente (`.env`)

Copie o arquivo `example.env` para `.env` e configure conforme seu ambiente:

```bash
cp example.env .env
```

| Variável | Padrão | Descrição |
| :--- | :--- | :--- |
| `PORT` | `8080` | Porta onde o servidor HTTP do painel irá rodar. |
| `GLPI_URL` | - | URL base do seu GLPI (ex: `https://seu-glpi.com.br`). |
| `GLPI_APP_TOKEN` | - | Token de Aplicação gerado no GLPI em *Configurar > Geral > API*. |
| `GLPI_USER_TOKEN` | - | Token Pessoal de API do usuário do GLPI. |
| `GLPI_TECH_GROUP` | `1` | ID do grupo técnico padrão (ex: `1` para Sistemas). |
| `REFRESH_INTERVAL_SEC` | `30` | Intervalo em segundos para atualização automática via AJAX. |
| `NOTIFICATION_SOUND_URL` | *vazio* | URL de arquivo `.mp3` para novos chamados gerais. Se vazio, usa som sintetizado padrão. |
| `CRITICAL_SOUND_URL` | *vazio* | URL de arquivo `.mp3` para chamados com prioridade Alta para cima. Se vazio, usa alarme sintetizado. |
| `CATEGORY_SOUNDS` | *vazio* | Regras de áudio específicas por categoria (veja detalhes abaixo). |
| `LOG_MAX_SIZE_MB` | `50` | Tamanho máximo em MB de cada arquivo de log rotativo. |
| `LOG_MAX_AGE_DAYS` | `30` | Período em dias para retenção dos logs gravados em disco. |

---

## 🔊 Configuração de Sons por Categoria (`CATEGORY_SOUNDS`)

Você pode associar áudios `.mp3` específicos para determinadas categorias de chamados. O sistema baixa os arquivos automaticamente na inicialização e faz cache local.

### Sintaxe:
Pares no formato `(Nome da Categoria, URL_DO_MP3)`. Múltiplas regras podem ser separadas por vírgula ou ponto e vírgula.

```env
# Exemplo com uma regra:
CATEGORY_SOUNDS="(Equipamentos e Acessórios > Perda / Roubo, https://www.myinstants.com/media/sounds/bolsonaro-pegaram-meu-telefone.mp3)"

# Exemplo com múltiplas regras:
CATEGORY_SOUNDS="(Equipamentos e Acessórios > Perda / Roubo, https://exemplo.com/alerta-roubo.mp3), (Protheus > Faturamento, https://exemplo.com/faturamento.mp3)"
```

> **Hierarquia do Som**:
> 1. **Regra de Categoria**: Se a categoria do chamado novo coincidir com `CATEGORY_SOUNDS`, toca o som da categoria.
> 2. **Som Crítico**: Se não coincidir com categoria, mas tiver prioridade Alta/Muito Alta/Crítica, toca `CRITICAL_SOUND_URL` (ou alarme agudo).
> 3. **Som Padrão**: Se for prioridade normal, toca `NOTIFICATION_SOUND_URL` (ou chime de aeroporto).

---

## 🐳 Executando com Docker

### Opção 1: Via Imagem Pronta do GitHub Packages (GHCR)

O arquivo `docker-compose.yml` já está configurado para consumir a imagem pronta oficial:

```yaml
services:
  glpi-painel:
    image: ghcr.io/robertocjunior/glpi-painel:latest
    container_name: glpi-painel
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .env
    volumes:
      - ./logs:/app/logs
```

Basta iniciar:
```bash
docker compose up -d
```

### Opção 2: Compilação Local com Docker

```bash
docker compose up -d --build
```

---

## 🖥️ Acesso ao Painel

Abra no navegador (ou configure na TV em modo tela cheia):
```text
http://localhost:8080
```

- Clique uma vez no botão **"Som Mudo / Som Ativo"** no canto superior direito para desbloquear o áudio no navegador.
