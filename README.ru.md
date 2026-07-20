# mcp-k8s-ephemeral-job

[English](README.md) | **Русский**

[![Version](https://img.shields.io/github/v/tag/inhuman/mcp-k8s-ephemeral-job?sort=semver&style=flat-square&label=version)](https://github.com/inhuman/mcp-k8s-ephemeral-job/tags)
[![Docker Pulls](https://img.shields.io/docker/pulls/idconstruct/mcp-k8s-ephemeral-job?style=flat-square&logo=docker)](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job)
[![Build](https://img.shields.io/github/actions/workflow/status/inhuman/mcp-k8s-ephemeral-job/docker-publish.yml?style=flat-square&logo=github)](https://github.com/inhuman/mcp-k8s-ephemeral-job/actions/workflows/docker-publish.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)

Публичный OSS MCP-сервер (Go, MIT), который поднимает **эфемерный Kubernetes Job/под** из
указанного вызывающей стороной образа, выполняет в нём команду, возвращает `exit_code` /
`stdout` / **артефакты** и **удаляет под**. Манифест пода сервер **собирает в коде** — вызывающая
сторона передаёт только параметры, никогда не сырой YAML.

Это «операционная» рядом со «скальпелем» [`mcp-exec`](https://github.com/inhuman/mcp-exec): там
один Python-файл исполняется в запертой песочнице без сети за миллисекунды, здесь — **полноценный
под** с выбранным вами образом и тулчейном, **управляемой сетью** (клонировать репы, тянуть
зависимости), долгими задачами и файловыми **артефактами** наружу. Они дополняют друг друга.

Работает на трёх транспортах — **stdio / HTTP / SSE** — с одинаковым набором инструментов везде
(официальный [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)).

## Инструменты

### `run_job` — запустить и дождаться

**Вход**: `{ image (обяз.), command (обяз.), files?, env?, limits?, timeout_s?, workdir?, clone? }`
**Выход**: `{ exit_code, stdout, stderr, duration_ms, status, artifacts, truncated }`

- `status` — одно из `succeeded` / `failed` / `timeout` / `error`. Ненулевой `exit_code` или
  `timeout` — **нормальный результат**, а не ошибка инструмента. Ошибка вызова — только невалидный
  вход (пустой image/command, образ вне аллоулиста, некорректный путь файла).
- `stdout` содержит **объединённые** stdout+stderr контейнера — Kubernetes сливает оба потока в
  логах пода. `stderr` зарезервирован и всегда пуст, поэтому разделение потоков можно будет
  добавить позже без слома совместимости.
- **Артефакты** (файлы из рабочей директории) возвращаются **инлайном** (base64) под капом по
  размеру. Превышение капа никогда не теряет данные молча: выставляется соответствующий флаг
  `truncated`.
- Манифест собирается сервером детерминированно — **сырого YAML от вызывающего нет**.

### `submit_job` / `fetch_job` — запустить в фоне

`submit_job` принимает те же аргументы, что `run_job`, но **сразу** возвращает `job_token`;
`fetch_job` забирает результат позже. Именно это позволяет агенту запустить долгую задачу (полную
батарею тестов, сборку) и продолжить работу, а не простаивать внутри одного синхронного вызова всё
её время.

- `fetch_job` возвращает `status=running`, пока джоба в полёте; передайте `wait_s` (≤120), чтобы
  долго-поллить вместо долбёжки. Ответ приходит сразу по завершении, а не досиживает весь `wait_s`.
- Результат хранится 60 минут и забирается больше одного раза.
- Аргументы валидируются на **submit**, поэтому неверный образ валит именно submit — пока вызывающий
  ещё может его исправить.
- Хэндлы живут в памяти (сервер по дизайну однорепличный): рестарт теряет незабранные токены,
  вызывающий просто отправляет задачу заново.

### Клонирование репозитория

С `clone: { repo_url, ref, subdir? }` init-контейнер выкладывает репозиторий в рабочую директорию
до старта команды. **Вызывающая сторона не касается кредов**: сервер держит секрет с токеном на
каждый git-хост, монтирует его **только** на клонер, а после клона токен маскируется в
`.git/config` — основной контейнер его не видит. Поле `clone` принимается, только если оператор
настроил `MCP_K8S_CLONE_IMAGE` + `MCP_K8S_CLONE_SECRET`.

## Модель безопасности (инварианты)

На каждый прогон — **свежий эфемерный под**, удаляемый после (успех / провал / таймаут). RBAC
сервера **ограничен одним namespace** (`Role`/`RoleBinding`, никогда `ClusterRole`):
`create/delete jobs,pods` плюс `pods/log`, `pods/exec` в **одном** namespace. Порождённые поды
работают с `cap-drop=ALL`, `no-privilege-escalation`, seccomp `RuntimeDefault`. Радиус поражения —
этот один namespace: `LimitRange` (дефолт+максимум на под) + `ResourceQuota` (потолок namespace) +
wall-clock таймаут (→ kill) + TTL/owner-reference GC + кап конкурентности. Образы обязаны пройти
**строгий аллоулист** (`MCP_K8S_ALLOWED_IMAGES`; пусто = не запускается ничего). Данные вызывающего
(`command` / `files` / вывод / артефакты) не персистятся и не логируются целиком — только метаданные.

> `run_job` — самая мощная поверхность из возможных (он создаёт поды). Встраивая его в агента,
> закрывайте его тул-политикой этого агента (только доверенные роли).

**Про сеть:** сеть пода **не** отключена (она нужна, чтобы клонировать репы и тянуть зависимости).
Egress ограничивается `NetworkPolicy` namespace (аллоулист) на уровне деплоя, а не правом,
зашитым в код. Инвариант — эфемерность и удаление, а не отсутствие сети.

**Ресурсы:** серверные `MCP_K8S_DEFAULT_CPU`/`MEMORY` — это **requests** пода (резерв планировщика).
Limits ставятся, только когда вызывающий передал `limits`; иначе потолок отдаёт `LimitRange`
namespace. Переданный `limits.memory` поднимает и request памяти: память несжимаема, и под обязан
сесть на ноду, где она реально есть.

## Запуск

Опубликован в [MCP Registry](https://registry.modelcontextprotocol.io) как
`io.github.inhuman/mcp-k8s-ephemeral-job`; образ — на Docker Hub:
[`idconstruct/mcp-k8s-ephemeral-job`](https://hub.docker.com/r/idconstruct/mcp-k8s-ephemeral-job).

```bash
docker run --rm -i \
  -v "$HOME/.kube:/kube:ro" \
  -e MCP_K8S_KUBECONFIG=/kube/config \
  -e MCP_K8S_NAMESPACE=ephemeral-dev \
  -e MCP_K8S_ALLOWED_IMAGES=busybox:1.36,python:3.12-slim \
  idconstruct/mcp-k8s-ephemeral-job:latest
```

Из исходников, против dev-кластера, по stdio:

```bash
go build -o mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job
export MCP_K8S_KUBECONFIG=$HOME/.kube/config
export MCP_K8S_NAMESPACE=ephemeral-dev
export MCP_K8S_ALLOWED_IMAGES=busybox:1.36,python:3.12-slim
./mcp-k8s-ephemeral-job
```

В проде сервер работает **внутри кластера** как Deployment со своим ServiceAccount +
`Role`/`RoleBinding` на эфемерный namespace + `ResourceQuota`/`LimitRange` (+ опционально egress
`NetworkPolicy`), обычно на транспорте `http`.

### Пример вызова (`run_job`)

```json
{
  "image": "python:3.12-slim",
  "command": ["python", "gen.py"],
  "files": [{ "path": "gen.py", "content_b64": "<base64 скрипта, пишущего out.png>" }],
  "limits": { "cpu": "500m", "memory": "256Mi" },
  "timeout_s": 30
}
```

Вернёт `exit_code`, захваченный вывод и `out.png` инлайном в `artifacts`. После этого пода уже нет
(`kubectl get jobs,pods -n $NS` пуст).

### Опциональная авторизация (HTTP/SSE)

Задайте `MCP_K8S_AUTH_TOKEN`, чтобы каждый HTTP/SSE-запрос обязан был нести совпадающий заголовок
`X-MCP-AUTH` (сравнение за константное время, иначе `401`). Пустой токен выключает проверку. К stdio
не применяется.

## Конфигурация

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `MCP_K8S_TRANSPORT` | `stdio` \| `http` \| `sse` | `stdio` |
| `MCP_K8S_ADDR` | адрес прослушивания для http/sse | `:8080` |
| `MCP_K8S_NAMESPACE` | namespace, где создаются эфемерные поды | `mcp-ephemeral` |
| `MCP_K8S_DEFAULT_TIMEOUT_S` | таймаут по умолчанию (с) | `60` |
| `MCP_K8S_MAX_TIMEOUT_S` | потолок таймаута (с) | `600` |
| `MCP_K8S_MAX_OUTPUT_BYTES` | кап объединённого stdout+stderr | `1048576` |
| `MCP_K8S_MAX_ARTIFACT_BYTES` | кап суммарного размера артефактов | `10485760` |
| `MCP_K8S_DEFAULT_CPU` | CPU request пода (резерв планировщика; limits — из `limits` вызова или LimitRange) | `1` |
| `MCP_K8S_DEFAULT_MEMORY` | memory request пода (см. выше) | `512Mi` |
| `MCP_K8S_MAX_CONCURRENT` | максимум одновременных эфемерных подов | `10` |
| `MCP_K8S_ALLOWED_IMAGES` | строгий аллоулист образов (CSV); **пусто = запрещено всё** | `` |
| `MCP_K8S_SIDECAR_IMAGE` | вспомогательный образ для сбора артефактов (запиненный) | `busybox:1.36` |
| `MCP_K8S_CLONE_IMAGE` | образ с git для init-клонера; пусто = `clone` недоступен | `` |
| `MCP_K8S_CLONE_SECRET` | секрет с токеном на каждый git-хост (ключ = хост); монтируется только на клонер | `` |
| `MCP_K8S_CACHE_PVC` | существующий PVC, монтируемый в каждый под как общий кеш; пусто = без кеша | `` |
| `MCP_K8S_CACHE_MOUNT_PATH` | куда монтируется кеш-PVC, напр. `/go/pkg/mod` | `` |
| `MCP_K8S_JOB_EXTRA_ENV` | JSON-объект `{"KEY":"value"}`, добавляемый каждому поду; ключи вызова важнее | `` |
| `MCP_K8S_KUBECONFIG` | путь к kubeconfig; пусто = in-cluster | `` |
| `MCP_K8S_AUTH_TOKEN` | если задан, http/sse требуют заголовок `X-MCP-AUTH`; пусто = выключено | `` |

Обе кеш-переменные должны быть заданы вместе, иначе кеш не монтируется. Сам PVC создаётся отдельно
(helm/манифест) — сервер только ссылается на него по имени и падает на старте, если его нет.

## Не реализовано

Отдача через PVC / объектное хранилище для артефактов, не влезающих инлайном, мульти-кластерность и
проксирование других MCP-серверов внутрь пода.

## Лицензия

MIT.
