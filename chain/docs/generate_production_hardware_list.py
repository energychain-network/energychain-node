#!/usr/bin/env python3
"""Generate EnergyChain production hardware / cloud server inventory Excel."""

from openpyxl import Workbook
from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
from openpyxl.utils import get_column_letter

OUTPUT = "EnergyChain_生产环境硬件清单.xlsx"

HEADER_FILL = PatternFill("solid", fgColor="1F4E79")
HEADER_FONT = Font(bold=True, color="FFFFFF", size=11)
SUBHEADER_FILL = PatternFill("solid", fgColor="D6E4F0")
TITLE_FONT = Font(bold=True, size=14)
BOLD = Font(bold=True)
THIN = Side(style="thin", color="BFBFBF")
BORDER = Border(left=THIN, right=THIN, top=THIN, bottom=THIN)
WRAP = Alignment(wrap_text=True, vertical="top")
CENTER = Alignment(horizontal="center", vertical="center", wrap_text=True)


def style_header_row(ws, row, cols):
    for c in range(1, cols + 1):
        cell = ws.cell(row=row, column=c)
        cell.fill = HEADER_FILL
        cell.font = HEADER_FONT
        cell.alignment = CENTER
        cell.border = BORDER


def style_table(ws, start_row, end_row, cols):
    for r in range(start_row, end_row + 1):
        for c in range(1, cols + 1):
            cell = ws.cell(row=r, column=c)
            cell.border = BORDER
            cell.alignment = WRAP


def set_col_widths(ws, widths):
    for i, w in enumerate(widths, 1):
        ws.column_dimensions[get_column_letter(i)].width = w


def add_title(ws, title, subtitle=None):
    ws["A1"] = title
    ws["A1"].font = TITLE_FONT
    if subtitle:
        ws["A2"] = subtitle
        ws["A2"].font = Font(color="666666", size=10)
        return 4
    return 3


wb = Workbook()

# ---------------------------------------------------------------------------
# Sheet 1: 架构总览
# ---------------------------------------------------------------------------
ws = wb.active
ws.title = "架构总览"
start = add_title(
    ws,
    "EnergyChain 生产环境架构总览",
    "链 ID: energychain-1 | 出块 ~3s | Cosmos SDK + EVM | 9 业务模块 | 2026-07 参考报价",
)
rows = [
    ["层级", "组件", "数量", "部署方式", "说明"],
    ["共识层", "Validator 验证者节点", "4", "独立物理/云主机", "参与共识与出块；pruning=nothing；私钥仅驻留本机；不对外暴露 RPC"],
    ["共识层", "Sentry 哨兵节点", "2", "独立云主机（建议）", "P2P 防火墙层；Validator 仅连接 Sentry；对外暴露 26656"],
    ["数据层", "Seed 种子节点", "2", "轻量云主机", "新节点发现入口；固定公网 IP；仅 P2P 26656"],
    ["数据层", "Full Node 全节点", "2", "独立云主机", "对外 RPC/REST/gRPC/EVM JSON-RPC；供 DEX/Explorer/第三方调用"],
    ["应用层", "DEX 服务栈", "1", "Docker Compose", "indexer + API(8081) + Web(3001) + Postgres + Redis + Nginx(8090)"],
    ["应用层", "区块浏览器服务栈", "1", "Docker Compose", "indexer + API(8080) + Web(3000) + Postgres + Redis"],
    ["应用层", "链官网 / 门户前端", "1", "静态/CDN 或轻量 VM", "品牌官网、文档、钱包引导；可与 Explorer 分离部署"],
    ["基础设施", "负载均衡 / 反向代理", "1", "SLB/Nginx", "HTTPS 终结、WAF、限流；统一域名入口"],
    ["基础设施", "监控告警", "1", "Prometheus + Grafana", "节点高度、磁盘、CPU、出块延迟、Indexer 滞后"],
    ["基础设施", "备份与日志", "1", "对象存储 + 快照", "Validator 密钥备份、链数据快照、DB 备份"],
]
for i, row in enumerate(rows, start=start):
    for j, val in enumerate(row, 1):
        ws.cell(row=i, column=j, value=val)
style_header_row(ws, start, 5)
style_table(ws, start, start + len(rows) - 1, 5)
set_col_widths(ws, [14, 22, 8, 22, 55])

# ---------------------------------------------------------------------------
# Sheet 2: 服务器详细清单
# ---------------------------------------------------------------------------
ws2 = wb.create_sheet("服务器详细清单")
start = add_title(ws2, "生产服务器详细清单（推荐配置）", "含 CPU/内存/磁盘/网络/端口/OS/软件栈")
headers = [
    "序号", "角色", "主机名示例", "数量", "vCPU", "内存(GB)", "系统盘(GB)",
    "数据盘(GB)", "磁盘类型", "带宽(Mbps)", "公网IP", "操作系统",
    "核心软件", "开放端口", "部署目录/备注", "月参考价(¥)",
]
for j, h in enumerate(headers, 1):
    ws2.cell(row=start, column=j, value=h)
style_header_row(ws2, start, len(headers))

servers = [
    ["1", "Validator", "ec-val-01 ~ ec-val-04", "4", "16", "64", "100", "2000",
     "NVMe ESSD PL1", "50", "否(经Sentry)", "Ubuntu 22.04 LTS",
     "energychaind + cosmovisor + systemd", "26656(仅Sentry)", "~/.energychaind",
     "pruning=nothing；LimitNOFILE=65535；禁止 SSH 密码登录；HSM/冷备份助记词", "2,800"],
    ["2", "Sentry", "ec-sentry-01 ~ 02", "2", "8", "32", "80", "500",
     "NVMe ESSD PL1", "200", "是(固定)", "Ubuntu 22.04 LTS",
     "energychaind (非验证者)", "26656", "P2P 入口；persistent_peers 指向 Validator",
     "1,200"],
    ["3", "Seed", "ec-seed-01 ~ 02", "2", "4", "8", "80", "200",
     "SSD", "100", "是(固定)", "Ubuntu 22.04 LTS",
     "energychaind (seed mode)", "26656", "seed_mode=true；indexer=kv 可关",
     "350"],
    ["4", "Full Node", "ec-full-01 ~ 02", "2", "16", "64", "100", "1500",
     "NVMe ESSD PL1", "200", "是", "Ubuntu 22.04 LTS",
     "energychaind + EVM JSON-RPC", "26657,1317,9090,8545,8546",
     "供 DEX/Explorer/钱包；可开 state-sync；pruning=default",
     "2,600"],
    ["5", "DEX 应用栈", "ec-dex-01", "1", "16", "64", "100", "800",
     "NVMe ESSD PL1", "200", "是", "Ubuntu 22.04 LTS",
     "Docker: postgres+redis+indexer+api+web+nginx", "3001,8081,8090",
     "~/energychain/dex/deploy；Cosmos indexer 索引 x/market 等",
     "2,400"],
    ["6", "区块浏览器栈", "ec-explorer-01", "1", "16", "64", "100", "1000",
     "NVMe ESSD PL1", "200", "是", "Ubuntu 22.04 LTS",
     "Docker: postgres+redis+indexer+api+web", "3000,8080",
     "~/energychain/explorer/deploy；全链+EVM+9模块索引",
     "2,400"],
    ["7", "官网前端", "ec-web-01", "1", "4", "8", "80", "100",
     "SSD", "100", "是(CDN更佳)", "Ubuntu 22.04 / 静态托管",
     "Nginx / Next.js SSR / OSS+CDN", "80,443",
     "品牌站、文档、下载页；建议 CDN 加速",
     "300"],
    ["8", "负载均衡/WAF", "ec-lb-01", "1", "4", "8", "40", "—",
     "—", "按量", "是", "云 SLB 或 Nginx",
     "SLB/Nginx + Certbot/Let's Encrypt", "80,443",
     "api.explorer / dex / rpc 子域名分流",
     "500"],
    ["9", "监控告警", "ec-mon-01", "1", "4", "16", "80", "500",
     "SSD", "50", "内网", "Ubuntu 22.04 LTS",
     "Prometheus + Grafana + Alertmanager", "9090,3000",
     "采集各节点 exporter；Telegram/钉钉告警",
     "450"],
    ["10", "备份存储", "对象存储", "1", "—", "—", "—", "2000",
     "对象存储/OSS", "—", "—", "云 OSS/S3",
     "定时快照 + genesis/validator 密钥加密备份", "—",
     "链数据 weekly 快照；DB daily 备份；保留 30 天",
     "400"],
]

r = start + 1
for row in servers:
    for j, val in enumerate(row, 1):
        ws2.cell(row=r, column=j, value=val)
    r += 1
style_table(ws2, start, r - 1, len(headers))
set_col_widths(ws2, [6, 12, 18, 6, 6, 8, 8, 8, 14, 10, 10, 14, 28, 18, 32, 12])

# ---------------------------------------------------------------------------
# Sheet 3: 云厂商实例映射
# ---------------------------------------------------------------------------
ws3 = wb.create_sheet("云厂商实例映射")
start = add_title(ws3, "主流云厂商实例规格映射", "价格因区域/活动/合约而异，下表为 2026 年参考区间（人民币/月）")
headers = [
    "角色", "阿里云 ECS", "腾讯云 CVM", "华为云 ECS", "AWS EC2 (新加坡)",
    "vCPU", "内存(GB)", "数据盘建议", "月价区间(¥)", "月价区间($)",
]
for j, h in enumerate(headers, 1):
    ws3.cell(row=start, column=j, value=h)
style_header_row(ws3, start, len(headers))

cloud = [
    ["Validator ×4", "ecs.g7.4xlarge", "SA5.4XLARGE64", "c7.4xlarge.2", "m6i.4xlarge",
     "16", "64", "2TB ESSD PL1", "2,600~3,200", "360~440"],
    ["Sentry ×2", "ecs.g7.2xlarge", "SA5.2XLARGE32", "c7.2xlarge.2", "m6i.2xlarge",
     "8", "32", "500GB ESSD", "1,000~1,400", "140~195"],
    ["Seed ×2", "ecs.c7.xlarge", "SA5.LARGE8", "c7.xlarge.2", "t3.xlarge",
     "4", "8", "200GB SSD", "280~420", "40~60"],
    ["Full Node ×2", "ecs.g7.4xlarge", "SA5.4XLARGE64", "c7.4xlarge.2", "m6i.4xlarge",
     "16", "64", "1.5TB ESSD PL1", "2,400~3,000", "330~415"],
    ["DEX 栈 ×1", "ecs.g7.4xlarge", "SA5.4XLARGE64", "c7.4xlarge.2", "m6i.4xlarge",
     "16", "64", "800GB ESSD", "2,200~2,800", "305~385"],
    ["Explorer 栈 ×1", "ecs.g7.4xlarge", "SA5.4XLARGE64", "c7.4xlarge.2", "m6i.4xlarge",
     "16", "64", "1TB ESSD", "2,200~2,800", "305~385"],
    ["官网前端 ×1", "ecs.c7.large", "SA5.MEDIUM4", "c7.large.2", "t3.large",
     "2~4", "4~8", "100GB SSD", "150~350", "20~50"],
    ["SLB/WAF", "ALB + WAF 按量", "CLB + WAF", "ELB + WAF", "ALB + WAF",
     "—", "—", "—", "400~800", "55~110"],
    ["监控 ×1", "ecs.c7.xlarge", "SA5.LARGE16", "c7.xlarge.2", "t3.xlarge",
     "4", "16", "500GB SSD", "350~550", "50~75"],
    ["对象存储 2TB", "OSS 标准", "COS 标准", "OBS 标准", "S3 Standard",
     "—", "—", "2TB", "300~500", "40~70"],
]
r = start + 1
for row in cloud:
    for j, val in enumerate(row, 1):
        ws3.cell(row=r, column=j, value=val)
    r += 1
style_table(ws3, start, r - 1, len(headers))
set_col_widths(ws3, [14, 18, 18, 16, 20, 8, 8, 14, 14, 12])

# ---------------------------------------------------------------------------
# Sheet 4: 费用汇总
# ---------------------------------------------------------------------------
ws4 = wb.create_sheet("费用汇总")
start = add_title(ws4, "月度 / 年度费用汇总", "基于推荐配置与中位参考价估算")
headers = ["费用类别", "明细", "数量", "单价(¥/月)", "小计(¥/月)", "小计(¥/年)", "备注"]
for j, h in enumerate(headers, 1):
    ws4.cell(row=start, column=j, value=h)
style_header_row(ws4, start, len(headers))

costs = [
    ["计算资源", "Validator 节点", "4", "2,800", "=D5*C5", "=E5*12", "核心共识，不可缩减"],
    ["计算资源", "Sentry 节点", "2", "1,200", "=D6*C6", "=E6*12", "建议 2 台跨可用区"],
    ["计算资源", "Seed 节点", "2", "350", "=D7*C7", "=E7*12", "可 1 主 1 备"],
    ["计算资源", "Full Node", "2", "2,600", "=D8*C8", "=E8*12", "RPC 高可用"],
    ["计算资源", "DEX 应用栈", "1", "2,400", "=D9*C9", "=E9*12", "含 Docker 全家桶"],
    ["计算资源", "Explorer 应用栈", "1", "2,400", "=D10*C10", "=E10*12", "全链索引"],
    ["计算资源", "官网前端", "1", "300", "=D11*C11", "=E11*12", "可用 OSS+CDN 降至 ¥100"],
    ["网络与安全", "SLB + WAF + 公网带宽", "1", "800", "=D12*C12", "=E12*12", "按 200Mbps 估算"],
    ["运维", "监控告警平台", "1", "450", "=D13*C13", "=E13*12", ""],
    ["存储", "对象存储与快照", "1", "400", "=D14*C14", "=E14*12", "2TB 标准存储"],
    ["一次性", "域名 + SSL 证书", "—", "—", "500", "500", "年费，多域名"],
    ["一次性", "HSM / 密钥管理(可选)", "—", "—", "3,000", "3,000", "Validator 密钥硬件保护"],
    ["人力(参考)", "DevOps / SRE 值守", "—", "—", "25,000", "300,000", "1 名全职，可按外包调整"],
]
r = start + 1
for row in costs:
    for j, val in enumerate(row, 1):
        ws4.cell(row=r, column=j, value=val)
    r += 1

# Subtotal rows
ws4.cell(row=r, column=1, value="月度基础设施合计").font = BOLD
ws4.cell(row=r, column=5, value="=SUM(E5:E14)").font = BOLD
ws4.cell(row=r, column=6, value="=SUM(F5:F14)").font = BOLD
r += 1
ws4.cell(row=r, column=1, value="含一次性首年合计(不含人力)").font = BOLD
ws4.cell(row=r, column=6, value="=F15+E16+E17").font = BOLD
r += 1
ws4.cell(row=r, column=1, value="含人力首年总预算(参考)").font = BOLD
ws4.cell(row=r, column=6, value="=F16+F18").font = BOLD

style_table(ws4, start, r, len(headers))
set_col_widths(ws4, [14, 22, 8, 12, 14, 14, 28])

# ---------------------------------------------------------------------------
# Sheet 5: 网络与安全
# ---------------------------------------------------------------------------
ws5 = wb.create_sheet("网络与安全")
start = add_title(ws5, "网络拓扑与安全基线")
items = [
    ["项目", "要求", "说明"],
    ["Validator 隔离", "仅内网/VPC 私网 IP", "Validator 不绑定公网 IP；26656 仅允许 Sentry 安全组"],
    ["Sentry 拓扑", "Validator ↔ Sentry ↔ 公网", "persistent_peers 单向；Sentry 可多 AZ"],
    ["Seed 节点", "固定公网 IP + DNS", "seed.explorer.yourdomain.com:26656"],
    ["Full Node RPC", "经 SLB + 限流 + API Key", "26657/1317/9090/8545/8546；禁止 admin 接口"],
    ["DEX 端口", "3001 Web / 8081 API / 8090 Nginx", "生产环境走 HTTPS 反代"],
    ["Explorer 端口", "3000 Web / 8080 API", "Indexer 7070 仅内网"],
    ["防火墙", "ufw / 安全组最小开放", "参考 deploy.sh：22,80,443,26656,26657,1317,8545,8546,9090,3000,3001,8080,8081,8090"],
    ["TLS", "Let's Encrypt 或云证书", "全站 HTTPS；HSTS"],
    ["密钥管理", "助记词离线 + 多签备份", "Validator 密钥禁止上云盘明文；考虑 HSM"],
    ["监控指标", "区块高度 / 磁盘 / peers / indexer lag", "告警：高度停滞 >30s、磁盘 >80%、Indexer 落后 >100 块"],
    ["备份策略", "DB 日备 + 链快照周备", "RPO 24h / RTO 4h 目标"],
    ["高可用", "Full×2 + Sentry×2 + Seed×2", "单点故障不影响出块与 RPC"],
]
for i, row in enumerate(items, start=start):
    for j, val in enumerate(row, 1):
        ws5.cell(row=i, column=j, value=val)
style_header_row(ws5, start, 3)
style_table(ws5, start, start + len(items) - 1, 3)
set_col_widths(ws5, [18, 35, 55])

# ---------------------------------------------------------------------------
# Sheet 6: 端口与服务清单
# ---------------------------------------------------------------------------
ws6 = wb.create_sheet("端口与服务清单")
start = add_title(ws6, "各组件端口与服务对照", "与 chain/scripts/prod/deploy.sh 及 apps.sh 一致")
headers = ["服务", "协议", "端口", "绑定地址", "用途", "是否公网暴露"]
for j, h in enumerate(headers, 1):
    ws6.cell(row=start, column=j, value=h)
style_header_row(ws6, start, len(headers))
ports = [
    ["Tendermint P2P", "TCP", "26656", "0.0.0.0", "节点间区块同步", "Sentry/Seed/Full 是；Validator 否"],
    ["Tendermint RPC", "HTTP", "26657", "0.0.0.0", "共识 RPC / status", "Full Node（限流）"],
    ["Cosmos REST", "HTTP", "1317", "0.0.0.0", "LCD REST API", "Full Node（限流）"],
    ["gRPC", "HTTP/2", "9090", "0.0.0.0", "Protobuf gRPC", "Full Node（内网/限流）"],
    ["EVM JSON-RPC", "HTTP", "8545", "0.0.0.0", "以太坊兼容 RPC", "Full Node（限流）"],
    ["EVM WebSocket", "WS", "8546", "0.0.0.0", "EVM 订阅", "Full Node（限流）"],
    ["Explorer Web", "HTTP", "3000", "0.0.0.0", "区块浏览器前端", "是（HTTPS 反代）"],
    ["Explorer API", "HTTP/WS", "8080", "0.0.0.0", "浏览器 REST + /ws", "是（HTTPS 反代）"],
    ["Explorer Indexer", "HTTP", "7070", "内网", "Indexer metrics", "否"],
    ["DEX Web", "HTTP", "3001", "0.0.0.0", "DEX 前端", "是（HTTPS 反代）"],
    ["DEX API", "HTTP/WS", "8081", "0.0.0.0", "DEX REST + /ws", "是（HTTPS 反代）"],
    ["DEX Nginx", "HTTP", "8090", "0.0.0.0", "DEX 静态/反代", "是（HTTPS 反代）"],
    ["Postgres (DEX)", "TCP", "5432", "Docker 内网", "DEX 数据库", "否"],
    ["Postgres (Explorer)", "TCP", "55433", "宿主机映射", "Explorer DB（避免冲突）", "否"],
    ["Redis", "TCP", "6379/56380", "Docker/映射", "缓存", "否"],
    ["SSH", "TCP", "22", "管理网", "运维", "堡垒机/IP 白名单"],
    ["HTTPS", "TCP", "443", "SLB", "统一入口", "是"],
]
r = start + 1
for row in ports:
    for j, val in enumerate(row, 1):
        ws6.cell(row=r, column=j, value=val)
    r += 1
style_table(ws6, start, r - 1, len(headers))
set_col_widths(ws6, [18, 10, 8, 12, 22, 28])

# ---------------------------------------------------------------------------
# Sheet 7: 部署检查清单
# ---------------------------------------------------------------------------
ws7 = wb.create_sheet("部署检查清单")
start = add_title(ws7, "上线前检查清单")
headers = ["阶段", "检查项", "负责角色", "状态"]
for j, h in enumerate(headers, 1):
    ws7.cell(row=start, column=j, value=h)
style_header_row(ws7, start, len(headers))
checks = [
    ["创世", "genesis.json SHA256 多方验证", "核心开发", "□"],
    ["创世", "4 验证者 gentx 收集并完成 03_finalize", "Validator 运营", "□"],
    ["网络", "Validator 无公网 IP，仅连 Sentry", "DevOps", "□"],
    ["网络", "Seed 固定 IP 写入文档与 default node config", "DevOps", "□"],
    ["节点", "cosmovisor + systemd ec-node 自启动", "DevOps", "□"],
    ["节点", "minimum-gas-prices=10000000000uecy", "DevOps", "□"],
    ["节点", "pruning nothing (Validator) / default (Full)", "DevOps", "□"],
    ["应用", "explorer + dex docker compose 迁移完成", "后端", "□"],
    ["应用", "INDEXER_START_HEIGHT 与链高度一致", "后端", "□"],
    ["安全", "防火墙/安全组最小端口", "安全", "□"],
    ["安全", "Validator 助记词离线备份 ≥3 份", "安全", "□"],
    ["监控", "Grafana 仪表盘 + 告警通道测试", "SRE", "□"],
    ["备份", "Postgres 自动备份到 OSS 验证恢复", "SRE", "□"],
    ["DNS", "rpc / explorer / dex 子域名解析", "DevOps", "□"],
    ["压测", "testpack.sh 全模块通过", "QA", "□"],
]
r = start + 1
for row in checks:
    for j, val in enumerate(row, 1):
        ws7.cell(row=r, column=j, value=val)
    r += 1
style_table(ws7, start, r - 1, len(headers))
set_col_widths(ws7, [12, 42, 14, 8])

out_path = __import__("pathlib").Path(__file__).resolve().parent / OUTPUT
wb.save(out_path)
print(f"Generated: {out_path}")
