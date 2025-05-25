🎞️ HLS Transcoder (Go + FFmpeg)
================================

**Este projeto foi criado com fins educacionais**, para aprender como funciona o processo completo de **transcodificação de vídeo adaptativa para HLS** utilizando `FFmpeg` e `Go`.

* * *

🚀 O que ele faz
----------------

*   Recebe um vídeo de entrada (`.mp4` ou `.ts`)
*   Gera múltiplos "flavors" (resoluções): de `2160p` a `180p`
*   Cria arquivos segmentados `.ts` e playlists `.m3u8` para cada resolução
*   Gera um **manifesto adaptativo** `master.m3u8` com todas as versões
*   Inclui um player HTML

![Fluxo](flow.png)

* * *

🛠️ Tecnologias usadas
----------------------

*   **Go** – servidor HTTP + execuções paralelas com goroutines
*   **FFmpeg** – transcodificação real dos vídeos
*   **HLS.js** – reprodução do vídeo HLS no navegador com seletor de qualidade
*   **Docker** - para rodar o projeto.

* * *

▶️ Como rodar
-------------

1.  Coloque um vídeo em `input/input.mp4` ou `input/input.ts`
2.  Execute o projeto com:
    `docker compose up`
3.  No navegador, acesse:
    *   `http://localhost:8080/transcode` → inicia a transcodificação
    *   `http://localhost:8080/index.html` → player

* * *

## ⚙️ Processamento Paralelo

Este projeto utiliza **execução paralela com goroutines** para acelerar o processo de transcodificação. Cada resolução de vídeo (ex: 2160p, 1080p, 720p, etc.) é processada simultaneamente, aproveitando ao máximo os núcleos da CPU disponíveis.

### 🔄 Como funciona:
- Cada tarefa de transcodificação é executada em uma **goroutine separada**.
- O uso de `sync.WaitGroup` garante que o programa aguarde a finalização de todas as resoluções antes de continuar.
- Isso permite ganhos significativos de performance, especialmente em servidores com múltiplos núcleos.

> ✅ Resultado: Transcodificações muito mais rápidas do que um processamento sequencial.

* * *

⚠️ Limitações
-------------

*   Funciona **apenas com HLS** (não gera arquivos para DASH, CMAF, WebM etc.)
*   Não faz upload via navegador (input deve estar no diretório local)
*   Não possui persistência ou banco de dados
*   Não aplica DRM

* * *

📚 Objetivo
-----------

Este projeto foi feito durante um estudo prático sobre transcodificação, streaming adaptativo (ABR), containers de mídia e servidores simples em Go.
