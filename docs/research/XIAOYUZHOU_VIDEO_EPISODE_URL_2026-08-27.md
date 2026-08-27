# 小宇宙视频播客单集 URL 怎么拿

日期：2026-08-27
状态：只读研究；未修改代码、配置或生产状态

> 不是当前生产行为。小宇宙 [软件许可及服务协议](https://www.xiaoyuzhoufm.com/agreement) 仍禁止未经许可探查服务、干扰正常运行。下文只记录其**公开网页已经在用的接口与字段**，不建议为此新增登录态、App 私有接口或持久化下载。

## 结论

1. **节目页 URL 没有变。** 视频单集和音频单集一样，都是 `https://www.xiaoyuzhoufm.com/episode/{eid}`。App 里的「分享 / 复制链接」仍是这一条。
2. **RSS 仍然只给音频。** 即使网页标明「视频播客」，`feed.xyzfm.space` 的 `<enclosure>` 仍是 `type="audio/mp4"` 的 `.m4a`。MagicPodcast 现有同步只会把这条写进 `medium_url`，**拿不到视频文件**。
3. **真正的视频地址不在 RSS，也不在单集 HTML 的直链字段里。** 公开页只告诉你「有视频」：`episode.video.available === true`。点「播放视频」后，网页会请求同源接口：
   `GET https://www.xiaoyuzhoufm.com/api/episodes/{eid}/video-playback`
4. **该接口返回短时签名的 HLS master，不是可长期保存的 mp4。** 未登录即可拿到，但清单路径带 `/hls/preview/`，本次只给出 480p；页面上的 `defaultQuality` 却是 720。过期后网页会重新请求接口刷新，不适合写入数据库当权威媒体地址。
5. **若只要「打开原节目」：继续用节目页 URL。** 若要「播视频文件」：必须在播放当下向 `video-playback` 取 HLS，并接受 preview / 过期 / 画质可能低于 App。不要把签名 URL 当 RSS enclosure 用。

## 三种容易混在一起的 URL

| 名称 | 是否稳定 | 本次样本 | 用途 |
| --- | --- | --- | --- |
| 节目页 | 稳定 | `https://www.xiaoyuzhoufm.com/episode/{eid}` | 打开原节目、分享、#197 恢复 |
| RSS / 网页音频 enclosure | 相对稳定（RSS 还包一层 `dts-api.xiaoyuzhoufm.com/track/...`） | `https://media.xyzcdn.net/{pid}/{file}.m4a`，`mimeType=audio/mp4` | 现有 `medium_url`、听音频 |
| 视频 HLS master | **不稳定**（`auth_key` + `expiresAt` / `ttl`） | `https://video.xyzcdn.net/episode-video/{videoId}/hls/preview/master.m3u8?auth_key=...` | 网页「播放视频」 |

`videoId` **不等于** `eid`。样本：`eid=6a734c29ab3a91c24a1067fa`，`videoId=6a7c52e0d30378f61e8da3cd`。

## 一手证据（2026-08-27）

样本节目：[罗永浩的十字路口](https://www.xiaoyuzhoufm.com/podcast/68981df29e7bcd326eb91d88)（`pid=68981df29e7bcd326eb91d88`）。Spotify 将该节目标为 Video；小宇宙网页单集按钮文案为「播放视频」，并带「视频播客」标识。

对照音频单集仍用此前的 E196：`eid=6a8cf80a1352af56ff3b7e2d`。

### 1. 公开页只声明「有视频」，不给文件地址

单集页 SSR / `_next/data/.../episode/{eid}.json` 中：

```json
"video": {
  "available": true,
  "cover": { "picUrl": "https://image.xyzcdn.net/....jpg", "width": 1920, "height": 1080 },
  "defaultQuality": { "id": "720", "width": 1280, "height": 720 }
}
```

同时仍有音频：

```json
"media": {
  "mimeType": "audio/mp4",
  "source": { "mode": "PUBLIC", "url": "https://media.xyzcdn.net/{pid}/....m4a" }
}
```

播客页最近单集列表里，每条也带同样的 `video.available`。页面静态资源包含 `video-podcast-brand` 图标。

音频对照（E196）没有可用的 `video.available` 播放链；其 `video-playback` 接口返回 **404** 正文 `Video playback not found`。

### 2. 网页播放走同源 `video-playback`

小宇宙单集页 JS（`pages/episode/[id]` chunk）在 `episode.video.available` 为真时，用 SWR 请求：

`/api/episodes/{eid}/video-playback`

未登录、浏览器 UA，对本样本返回 **200**：

```json
{
  "videoId": "6a7c52e0d30378f61e8da3cd",
  "duration": 10460,
  "playback": {
    "type": "hls",
    "master": {
      "url": "https://video.xyzcdn.net/episode-video/{videoId}/hls/preview/master.m3u8?auth_key=<REDACTED>",
      "expiresAt": 1787837636953,
      "ttl": 14199
    }
  },
  "qualities": [{ "id": "480", "width": 852, "height": 480 }]
}
```

要点：

- `playback.type` 为 `hls`，不是单个 mp4。
- 路径含 **`/hls/preview/`**；未登录 `qualities` 只有 480，和页上宣传的 720 不一致。
- `ttl` 约 4 小时；网页在 `Date.now() >= expiresAt` 时会重新请求该接口。
- 播放器用原生 HLS 或 hls.js 加载 `playback.master.url`。

点「播放视频」时，埋点为 `h5_play_click`，`content_info.status=video`，`web_info.campaign=episode_video`。未登录不会再打到别的媒体域，直到拿到上述 master。

### 3. HLS master 里有视频轨和音频轨

对返回的 preview master 做 **GET 播放列表文本**（不拉分片）：

```
#EXT-X-MEDIA:TYPE=AUDIO,... URI="audio/index.m3u8?auth_key=..."
#EXT-X-STREAM-INF:RESOLUTION=852x480,... URI="video/480p/index.m3u8?auth_key=..."
#EXT-X-STREAM-INF:... CODECS="mp4a.40.2" URI="audio/index.m3u8?auth_key=..."
```

即：同一套签名清单里同时有 480p 视频和纯音频变体。根路径 `https://video.xyzcdn.net/` 无路径 HEAD 为 CDN 403，正常。

### 4. RSS 不含视频

Apple 目录 `collectionId=1834069371` 指向 `https://feed.xyzfm.space/wmnkvmrpwuww`。本次 Feed **200**，36 条 `<enclosure>` 全部 `type="audio/mp4"`，无 `video/mp4`、无 `media:content`、无 m3u8。最新一集 enclosure 形如：

`https://dts-api.xiaoyuzhoufm.com/track/{pid}/{eid}/media.xyzcdn.net/{pid}/{file}.m4a`

这与网页 `media.source.url` 指向的 `media.xyzcdn.net` 音频是同一份声音，只是 RSS 多了一层 track 跳转。

MagicPodcast 同步仍取 `item.Enclosures[0]` 写入 `medium_url`（源码注释为「音频文件 URL」）。对视频节目，这条继续是 **音频**。

## 推荐获取步骤（只读、按需）

只想打开原节目：

1. 用已有 `episode.Link` / `original_url`（即 `/episode/{eid}`）。
2. 需要识别是不是视频单集时，看公开 JSON 的 `video.available`，不要猜 RSS MIME。

需要在浏览器里播网页同款视频：

1. 确认 `GET /api/episodes/{eid}/video-playback` 为 200（音频单集是 404）。
2. 用返回的 `playback.master.url` 交给 HLS 播放器。
3. 过期后重新请求接口，**不要把该 URL 写入 SQLite 当权威媒体**。

需要声音（转写、现有加工链）：

- 继续用 RSS / 网页的 **m4a 音频 enclosure**。视频节目仍然提供公开音频。

不要做：

- 为拿 720/全片去登 App、带 cookie 打未公开接口。
- 把 `/hls/preview/` 当成「官方全分辨率片源」。
- 把签名 HLS 写进 `medium_url` 或当 Feed enclosure。

## 和当前产品的关系

| 现有能力 | 对视频单集 |
| --- | --- |
| #197 打开原节目页 + 403 恢复 | 仍适用，节目页 URL 不变 |
| RSS 同步 `medium_url` | 仍是音频 m4a |
| 播放按钮 `window.open(medium_url)` | 打开的是音频文件，不是视频播放器 |
| 图片代理白名单 | 无 `video.xyzcdn.net`；本次也不应把 HLS 当封面代理 |

若以后要在站内区分「可看视频」，最小充分做法是：对小宇宙单集按需读 `video.available` 或 `video-playback` 的 200/404，而不是扩 schema 存过期 m3u8。是否做产品能力需另开票，本文件不授权实现。

## 来源

1. 小宇宙单集页与播客页 SSR / `_next/data`：`episode.video`、`episode.media`（样本 `eid=6a734c29ab3a91c24a1067fa`，`pid=68981df29e7bcd326eb91d88`）
2. 小宇宙网页 JS `pages/episode/[id]`：SWR 请求 `/api/episodes/{eid}/video-playback`，用 `playback.master.url` + hls.js
3. 未登录 `GET https://www.xiaoyuzhoufm.com/api/episodes/{eid}/video-playback`：视频单集 200 HLS preview；E196 音频单集 404 `Video playback not found`
4. `https://feed.xyzfm.space/wmnkvmrpwuww`：仅 `audio/mp4` enclosure
5. Apple 目录 `collectionId=1834069371` → 上述 Feed
6. [小宇宙软件许可及服务协议](https://www.xiaoyuzhoufm.com/agreement)（2022-09-01 公开版）3.9 / 3.10
7. MagicPodcast `backend/internal/sync/podcast_helpers.go` 将 `Enclosures[0]` 写入 `medium_url`；`backend/internal/models/episode.go` 注释为音频 URL
