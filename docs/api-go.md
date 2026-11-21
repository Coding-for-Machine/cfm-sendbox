# cfm-sendbox API (Go, Fiber v3)

[engineer-man/piston](https://github.com/engineer-man/piston) API'ning Go (Fiber v3 + contrib/v3/websocket) da noldan yozilgan varianti.
Asl Node.js `api/src` kodining barcha qismlari ko'chirilgan: config, runtime registry, package manager (ppman),
isolate asosidagi job executor, `/api/v2` REST endpointlari va `/api/v2/connect` WebSocket.

Asl loyiha MIT litsenziyasida.

## Struktura

```
cmd/sendbox-api/main.go      # Fiber v3 app, routelar, graceful shutdown (GracefulContext)
cmd/sendbox/                 # Go CLI (piston)
cmd/sendbox-repo/            # paket index serveri
internal/config/             # PISTON_* env o'zgaruvchilari, limit_overrides
internal/globals/            # versiya, signallar jadvali, platforma aniqlash
internal/runtime/            # o'rnatilgan runtime'lar reyestri, semver tanlash
internal/pkgman/             # repo index, yuklab olish, sha256, tar, o'rnatish/o'chirish
internal/job/                # isolate box, compile/run bosqichlari, limitlar, WebSocket bus
internal/api/                # /api/v2 handlerlar (execute, runtimes, packages, connect)
internal/cli/                # CLI: client, commands, semver, term, ws
docker/api/, docker-compose.yaml
```

## Endpointlar

| Metod | Yo'l | Tavsif |
|---|---|---|
| GET | `/healthz` | `ok` (Docker HEALTHCHECK) |
| GET | `/` | `{"message":"Piston v3.1.1"}` |
| GET | `/api/v2/runtimes` | o'rnatilgan tillar |
| POST | `/api/v2/execute` | kodni ishga tushirish (asl bilan bir xil request/response) |
| GET | `/api/v2/packages` | repo'dagi paketlar va `installed` holati |
| POST | `/api/v2/packages` | paket o'rnatish `{"language","version"}` |
| DELETE | `/api/v2/packages` | paketni o'chirish |
| WS | `/api/v2/connect` | interaktiv bajarish (`init`, `data`, `signal`) |

## Ishga tushirish

Talablar: Docker, cgroup v2 (cgroup v1 o'chirilgan bo'lishi kerak).

```bash
go mod tidy                 # Fiber v3 / contrib websocket ni yuklaydi, go.sum ni hosil qiladi
docker compose up -d --build

# til o'rnatish
curl -X POST localhost:2000/api/v2/packages \
  -H 'Content-Type: application/json' \
  -d '{"language":"python","version":"3.x"}'

# kod ishga tushirish
curl -X POST localhost:2000/api/v2/execute \
  -H 'Content-Type: application/json' \
  -d '{"language":"python","version":"*","files":[{"content":"print(\"salom\")"}]}'
```

Sozlamalar asl nusxadagidek `PISTON_` prefiksli env o'zgaruvchilar orqali beriladi
(`CFM_RUN_TIMEOUT`, `CFM_MAX_CONCURRENT_JOBS`, `CFM_REPO_URL`, `CFM_LIMIT_OVERRIDES`, ...).

## Asldan farqlar

- `run_timeout`, `run_cpu_time` va shunga o'xshash limitlarga `0` yuborilsa, "cheksiz" emas, sozlangan default ishlatiladi.
- WebSocket'da `signal` xabari haqiqatan jarayonga yuboriladi (asl kodda event nomi mos kelmagani uchun ishlamas edi); klient uzilsa jarayon to'xtatiladi.
- Noto'g'ri `base64`/`hex` kontent, fayl nomi orqali papkadan chiqib ketish, `args`/`stdin` turi xatosi `500` emas, `400` qaytaradi.
- Paketni o'chirish `provides` bilan ko'p tilli paketlarning barcha runtime'larini reyestrdan olib tashlaydi.
- Repo index yuklanmasa API `500` va `{"message": ...}` qaytaradi.
