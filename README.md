# QIANWEN-CREATOR-01 / qwen-create2api

`QIANWEN-CREATOR-01` is a standalone Web reverse-proxy provider for `create.qianwen.com`.

It is intentionally separated from `QIANWEN-WEB-01`:

- `QIANWEN-WEB-01` targets the main `www.qianwen.com` chat/video path.
- `QIANWEN-CREATOR-01` targets the Creator / AI Studio path and exists to package Web-only video creation features such as first-frame and first+last-frame video.

This project does not use official Qwen API keys. It stores logged-in Web session cookies in SQLite and exposes OpenAI-style media endpoints.

## Current Scope

| Capability | Status |
|---|---|
| SQLite account pool | Implemented |
| Admin WebUI | Implemented |
| QR/browser login capture | Opens `create.qianwen.com`, defaults to QR login, supports screenshot click/type controls, and captures Creator-related cookies |
| Async video task API | Implemented |
| Text-to-video | Historically verified on SH01 with `qianwen-creator-wan25-t2v`; requires a current `valid` account |
| First-frame video | Historically verified on SH01 with `first_frame_material_id` and `qianwen-creator-wan25-i2v`; requires a current `valid` account |
| First+last-frame video | Verified on SH01 with `qianwen-creator-wan22-flash-frame`, public first/last image URLs, and 720P 5s output |
| Public image URL to material id | Implemented through the observed Creator OSS upload flow: `/1/oss_token` -> OSS `PUT` -> `/1/oss/callback` |
| Data URI image upload | Implemented for `data:image/...;base64,...` values |
| Creator image generation | Not exposed in `/v1/models` yet |

Important: the Creator Web app uses client-side signing helpers before calling AI Studio endpoints. This repository implements the observed direct HTTP signing path for both video submit/query and Creator Resource image upload. Production validation still requires a current logged-in Creator Web account.

## Models

| Public model | Creator scene | Upstream model | First frame | Last frame |
|---|---|---|---:|---:|
| `qianwen-creator-wan21-frame` | `frame_image_to_video` | `wanx2.1-kf2v-plus` | yes | yes |
| `qianwen-creator-wan22-flash-frame` | `wan22_flash_frame_itv` | `wan2.2-kf2v-flash` | yes | yes |
| `qianwen-creator-wan25-i2v` | `wan25_first_frame_itv` | `wan2.5-i2v-preview` | yes | no |
| `qianwen-creator-wan25-t2v` | `wan25_txt_to_video` | `wan2.5-t2v-preview` | no | no |
| `qianwen-creator-wan27-frame` | `wan27_frame_i2v` | `wan2.7-i2v` | yes | yes |
| `qianwen-creator-happyhorse-i2v` | `hh_first_frame_i2v` | `happyhorse` | yes | no |

Code default video model: `qianwen-creator-wan22-flash-frame`.
SH01 currently runs with `DEFAULT_VIDEO_MODEL=qianwen-creator-wan25-t2v` for text-to-video smoke compatibility.

## gen2api Compatibility

`QIANWEN-CREATOR-01` is designed to be consumed by gen2api and NewAPI-style video gateways as a standalone provider. It must not be merged into `QIANWEN-WEB-01`.

| Integration Point | Contract |
|---|---|
| Provider code | `QIANWEN-CREATOR-01` |
| Default port | `18012` |
| API base | `/v1` |
| Runtime API key env | `QIANWEN_CREATOR_AUTH_KEY` in gen2api/deploy runtime files; mapped to service `AUTH_KEY` |
| Runtime admin key env | `QIANWEN_CREATOR_ADMIN_KEY` in gen2api/deploy runtime files; mapped to service `ADMIN_KEY` |
| Account sync source | `/opt/ai-aggregator/data/QIANWEN-CREATOR-01/qianwen-creator-01.sqlite`, table `qianwen_accounts` |
| Preferred manual test endpoint | `POST /v1/video/generations/sync` |
| Async gateway endpoint | `POST /v1/video/generations` then `GET /v1/video/generations/{task_id}` |
| First-frame field | `first_frame_image` |
| Last-frame field | `last_frame_image` |

gen2api playground behavior:

- `firstFrameUrl` is converted to `first_frame_image`.
- `lastFrameUrl` is converted to `last_frame_image`.
- Public URLs and data URI images are accepted; this service uploads them to Creator OSS and submits the returned material ids to create.qianwen.com.
- The third-party account page syncs saved Creator Admin WebUI accounts from SQLite. It does not store API keys, NewAPI channel keys, or raw Creator cookies as account materials.

## API

### Create Async Video

```bash
curl -X POST "$BASE_URL/v1/video/generations" \
  -H "Authorization: Bearer $AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qianwen-creator-wan22-flash-frame",
    "prompt": "A white cube rotates slowly on a table, realistic photography, five seconds.",
    "duration": 5,
    "resolution": "720P",
    "aspect_ratio": "9:16",
    "first_frame_image": "https://example.com/first-frame.png",
    "last_frame_image": "https://example.com/last-frame.png"
  }'
```

Response:

```json
{
  "id": "task-id",
  "task_id": "task-id",
  "object": "video.generation.task",
  "provider": "QIANWEN-CREATOR-01",
  "status": "queued",
  "poll_url": "/v1/video/generations/task-id"
}
```

### Poll Video

```bash
curl "$BASE_URL/v1/video/generations/$TASK_ID" \
  -H "Authorization: Bearer $AUTH_KEY"
```

### Synchronous Video

```bash
curl -X POST "$BASE_URL/v1/video/generations/sync" \
  -H "Authorization: Bearer $AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qianwen-creator-wan22-flash-frame",
    "prompt": "A white cube rotates slowly on a table, realistic photography, five seconds.",
    "duration": 5,
    "resolution": "720P",
    "aspect_ratio": "9:16",
    "first_frame_image": "https://example.com/first-frame.png",
    "last_frame_image": "https://example.com/last-frame.png"
  }'
```

`first_frame_image` and `last_frame_image` accept:

- Creator material ids.
- Public image URLs. The service downloads the image and uploads it to Creator Resource using the observed OSS flow before submitting the video task.
- Data URI values such as `data:image/png;base64,...`.

`first_frame_material_id` and `last_frame_material_id` remain supported for callers that already have Creator material ids.

## Admin

```text
/admin?key=<ADMIN_KEY>
```

Admin supports:

- Account list.
- Start Creator QR/screenshot login session.
- Click the remote Chromium screenshot, type text into the focused field, and send Enter/Tab/Backspace/Escape.
- Import Cookie Header / Cookie JSON for an already logged-in Creator browser session.
- Capture cookies into SQLite.
- Test account session.
- View task list.

## Docker

```bash
docker build -t qianwen-creator-01:latest .
docker run -d --name qianwen-creator-01 \
  -p 18012:8000 \
  -e HOST=0.0.0.0 \
  -e PORT=8000 \
  -e AUTH_KEY=change-me-api-key \
  -e ADMIN_KEY=change-me-admin-key \
  -e POOL_SIZE=0 \
  -e DATA_DIR=/app/data \
  -e DATABASE_PATH=/app/data/qianwen-creator-01.sqlite \
  -v ./data:/app/data \
  qianwen-creator-01:latest
```

## Environment

| Variable | Default |
|---|---|
| `HOST` | `0.0.0.0` |
| `PORT` | `8080` |
| `AUTH_KEY` | empty |
| `ADMIN_KEY` | `AUTH_KEY` |
| `DATA_DIR` | `./data` |
| `DATABASE_PATH` | `./data/qianwen-creator-01.sqlite` |
| `DEFAULT_VIDEO_MODEL` | `qianwen-creator-wan22-flash-frame` |
| `POOL_SIZE` | `0` |
| `BROWSER_HEADLESS` | `true` (`false` in `Dockerfile.novnc`) |
| `NOVNC_URL` | empty |

## Protocol Notes

Observed Creator frontend version:

- Page: `https://create.qianwen.com/r/ai-studio-pc/main/gen-video`
- Bundle: `https://g.alicdn.com/h5-pages/ai-studio-pc/0.6.33/csr/js/main.50f12c.js`

Observed video submit:

- Base: `https://ai-studio-create.qianwen.com`
- Path: `POST /api/web/ai/video/function`

Observed video query:

- Base: `https://ai-studio-create.qianwen.com`
- Path: `POST /api/web/ai/video/record/query`
- Body: `{ "recordId": "...", "scene": "..." }`

Observed image material upload:

- Base: `https://aistudio-resource.qianwen.com`
- Step 1: `POST /1/oss_token` with file name, MD5, size, file type, and `entry=ugc`.
- Step 2: `PUT` the image bytes to the returned OSS host/object with the returned OSS authorization headers.
- Step 3: `POST /1/oss/callback` with object, bucket, MD5, file type, and endpoint. The response returns `material_id`.
- The old `/1/material/file_url/restore` path is not used because direct HTTP restore returns upstream `code=10009` signature verification failures.

Observed first+last frame payload shape:

```json
{
  "originPrompt": "...",
  "prompt": "...",
  "scene": "wan22_flash_frame_itv",
  "model": "wan2.2-kf2v-flash",
  "rootModel": "wan22_flash",
  "genMode": "vid_gen",
  "params": {
    "size": "9:16",
    "resolution": "720P",
    "audio": false,
    "duration": 5,
    "attachmentType": 0,
    "attachments": [
      { "type": "image", "materialId": "first-frame-material-id" },
      { "type": "image", "materialId": "last-frame-material-id" }
    ]
  }
}
```

## SH01 Validation

Validated on `http://150.158.144.62:18012`:

- 2026-06-17 login regression: Admin loads, new-account flow opens a readable QR login screenshot by default, screenshot-click API works, and historical false-captured visitor accounts were removed.
- SH01 currently has a valid Creator login account. For new deployments, add a Creator account and run account test until it is marked `valid` before routing traffic.
- Historical: `qianwen-creator-wan25-t2v` text-to-video completed and returned a playable video URL.
- Historical: `qianwen-creator-wan25-i2v` first-frame image-to-video completed when using an existing Creator material id.
- 2026-06-18: `qianwen-creator-wan22-flash-frame` first+last-frame video completed using two public image URLs. The service uploaded both images through Creator OSS and returned a playable mp4.
- 2026-06-18 gen2api path: JP01 generated two related storyboard frames with `gpt-image-2`; SH01 submitted those images as data URIs to `qianwen-creator-wan22-flash-frame`; task `ba33dbb7-a7fd-485e-a18c-91bbe19aafd5` completed and returned a playable 5-second H.264 mp4.

## 账户检修（私有 noVNC）

`Dockerfile.novnc` 保留现有 API、SQLite 和账号池结构，只增加 headed Chromium 的
交互入口。账户 profile 固定为 `DATA_DIR/account-chrome-profiles/<account-id>`；活动
维护租约会将该账户排除出任务调度。设置 `VNC_PASSWORD` 和 `NOVNC_URL`，并且只把
容器 `6080` 映射到宿主机 `127.0.0.1`。

维护接口为 `/api/accounts/{id}/maintenance/{start|heartbeat|stop|validate}`，Cookie
捕获沿用 `/api/login-sessions/{lease-owner}/capture`。维护会话不能刷新重建，需停止后
重新开始，以保证 profile 与数据库租约仍由同一会话持有。
