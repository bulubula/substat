# Substat

轻量、极速、零外部依赖的原生子路径（Subpath-Native）服务可用性监控面板。

专为隐藏部署在 Nginx/Caddy 反代子路径（例如 `https://yourdomain.com/zymstat/`）设计，解决传统监控工具对二级目录路径支持繁琐、打包体积庞大等痛点。

---

## ✨ 核心特性

- **子路径原生友好（Subpath Native）**：底层内置 `http.StripPrefix` 与动态 `<base href>` 注入，任意子路径即配即用，无需繁琐改写资源规则。
- **单静态二进制（Zero Dependencies）**：前端 HTML/CSS/JS 经 `//go:embed` 直接内嵌进单一可执行文件，内存占用极小。
- **开箱即用免密面板（No Auth Required）**：定位内网与私密子路径，无繁重用户系统，直接展示服务状态。
- **可插拔探针机制（Pluggable Probes）**：基于适配器模式，探针执行与核心调度解耦：
  - `http`：单次 HTTP/HTTPS 状态码、响应头、正文正则与 TLS 证书天数检测。
  - `http_chain`：多步骤声明式链（支持模拟登录、Token 提取传递与后续鉴权探活）。
  - `tcp`：四层端口连通性及原生 TLS 握手与证书到期检测。
  - `dns`：直连指定 DNS 服务器解析探活与时延计算。
  - `ping`：ICMP Ping 往返延迟（RTT）与丢包率检测。
- **严格周期范围约束**：
  - 仅支持 `m`（分钟）、`h`（小时）、`d`（天）为单位。
  - 探测周期范围严格限定在 `1m` ~ `30d`（最大 43200 分钟），非法单位或超限自动拒绝启动。
- **可插拔告警通道（Alert Notifiers）**：
  - 支持 `Bark`、`Webhook` 等通知渠道。
  - 支持连续失败防抖阈值（`consecutive_failures`），状态翻转时异步推送。
- **双层紧凑时序存储**：
  - **热数据**：内存 RingBuffer 环形缓存（最近 60 点位），页面毫秒级秒开。
  - **冷数据**：极轻量结构化追加归档。

---

## 🚀 快速开始

### 1. 编译构建

```bash
cd substat
# 静态编译单二进制
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/substat ./cmd/substat
```

### 2. 运行

#### 二进制直接运行
```bash
cp config.example.yaml config.yaml
# 根据实际情况修改 config.yaml
./bin/substat -config config.yaml
```

#### Docker 容器运行
```bash
docker run -d \
  --name substat \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /path/to/config.yaml:/app/config.yaml:ro \
  -v /path/to/data:/app/data \
  ghcr.io/bulubula/substat:latest
```

访问 `http://127.0.0.1:8080/zymstat` 即可直接查看监控状态。

---

## ⚙️ Nginx 反代配置参考

隐藏部署在二级子路径，其余路径返回 444 防探测：

```nginx
server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    # 根路径及未匹配路径直接断开连接
    location / {
        return 444;
    }

    # 隐蔽子路径反向代理
    location /zymstat/ {
        proxy_pass http://127.0.0.1:8080/zymstat/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

---

## 📄 配置文件说明

完整示例参考 `config.example.yaml`。

```yaml
server:
  listen: "0.0.0.0:8080"
  base_path: "/zymstat"

storage:
  path: "./data/substat_history.jsonl"
  ring_buffer_size: 60

alerts:
  - id: "bark-admin"
    type: "bark"
    url: "https://api.day.app/YOUR_BARK_KEY"

monitors:
  - name: "trilium-wiki"
    interval: "5m"            # 仅允许 m, h, d，范围 1m ~ 30d
    timeout: "5s"
    consecutive_failures: 3   # 连续失败 3 次才报警
    alerts: ["bark-admin"]
    probe:
      type: "http"
      target: "http://192.168.0.5:6803/api/health"
      assert:
        - target: "status"
          operator: "equals"
          value: "200"
        - target: "latency_ms"
          operator: "lte"
          value: "1000"
```
