# GLPI-PAINEL

Painel autônomo e de alta performance em **Go** para monitoramento em tempo real de chamados da fila do GLPI (com foco na fila de Sistemas).

## Recursos

- **Backend em Go**: Conexão nativa com a API REST do GLPI (`apirest.php`), inicializando e destruindo sessões por ciclo de forma segura e eficiente.
- **Frontend AJAX embutido**: Sem dependências pesadas, HTML/CSS/JS servidos diretamente pelo próprio binário Go (`embed.FS`).
- **Tema Escuro & Design Limpo**: Cantos retos (0px), sem degradês, interface inspirada em consoles de monitoramento.
- **Alerta Sonoro de Aeroporto**: Efeito sonoro característico de chamada de aeroporto (ding-dong harmônico F5 -> C5) sintetizado em tempo real pelo navegador via **Web Audio API** (sem necessidade de arquivos de áudio externos ou dependências quebradas).
- **Métricas e Filtros**: Contadores rápidos de chamados Novos, Em Atendimento, Pendentes e busca instantânea por texto.
- **Container Único**: Execução estável via Docker multi-stage build.

## Como Executar com Docker

1. Configure as variáveis no arquivo `.env` (ou utilize `example.env` como base):
   ```env
   PORT=8080
   GLPI_URL=https://grupolipetral.verdanadesk.com
   GLPI_APP_TOKEN=seu_app_token
   GLPI_USER_TOKEN=seu_user_token
   GLPI_TECH_GROUP=1
   ```

2. Suba o container:
   ```bash
   docker compose up -d --build
   ```

3. Acesse no navegador:
   ```
   http://localhost:8080
   ```
