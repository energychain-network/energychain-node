# EnergyChain 原生功能蓝图

> 本文档面向产品、业务、合规、运营同事，用于对齐"链上提供哪些原生能力、各自解决什么问题、彼此如何协作"。
> 技术细节（proto 字段、消息名、keeper 接口签名）在各模块代码内的 doc 注释中展开，本文不涉及。
>
> **2026 重构说明**：早期蓝图曾规划 22 个细粒度模块。经"激进合并"后，链上现有 **9 个原生模块**——功能不减，但边界更清晰、跨模块调用更少、攻击面更小。本文已按合并后的实际代码（`chain/x/`）更新。旧 22 模块到新 9 模块的映射见 [第 8 节](#8-从-22-模块到-9-模块的合并映射)。

---

## 目录

1. [我们要解决什么问题](#1-我们要解决什么问题)
2. [EnergyChain 的定位](#2-energychain-的定位)
3. [整体架构（合并后）](#3-整体架构合并后)
4. [模块功能清单（9 个）](#4-模块功能清单9-个)
   - [identity —— 合规基座](#41-identity--合规基座didkyc--制裁--策略--审计)
   - [assethub —— 数据可信层](#42-assethub--数据可信层设备--计量--喂价--罚没)
   - [stableusd —— 多币种合规稳定币](#43-stableusd--多币种合规结算稳定币)
   - [rwatoken —— 合规资产通证](#44-rwatoken--合规资产通证)
   - [mincast —— 动态地板价做市](#45-mincast--动态地板价做市)
   - [offering —— 初始 RWA 发行](#46-offering--初始-rwa-发行iro)
   - [market —— 频繁批量拍卖撮合](#47-market--频繁批量拍卖撮合)
   - [automation —— 链上 Cron + 流式支付](#48-automation--链上-cron--流式支付)
   - [bridge —— 跨链结算资产桥](#49-bridge--跨链结算资产桥)
5. [十条通用设计原则](#5-十条通用设计原则)
6. [标准对齐](#6-标准对齐)
7. [分阶段路线图](#7-分阶段路线图)
8. [从 22 模块到 9 模块的合并映射](#8-从-22-模块到-9-模块的合并映射)
9. [名词表](#9-名词表)

---

## 1. 我们要解决什么问题

把全球能源 / 实物资产（RWA）收益权往链上搬，长期卡在几件事：

| 痛点 | 现状 | EnergyChain 的解法（一句话） |
|---|---|---|
| **数据不互信** | 装机出力、电表读数、储备金证明由不同主体上报，互相不认账 | 设备级签名 + 多源喂价聚合 + 可罚没的数据提供者（`assethub`） |
| **资产形态割裂** | 收益权、绿证、股权份额各成孤岛，跨平台流转必须线下确权 | 统一的合规通证层 `rwatoken`，全部走相同的身份/制裁/策略通道 |
| **结算非原子** | 现金、资产、对价在不同系统，无法"一手交钱一手交货" | 链上隔离账本稳定币 `stableusd` + 撮合即结算（`market`） |
| **二级流动性差** | 收益权代币发完即"躺平"，缺乏价格发现 | `mincast` 把交易手续费持续注入底池，地板价随交易频次抬升 |
| **监管与合规冲突** | 既要 KYC / 制裁过滤，又要可治理升级 | `identity` 把身份、制裁、转让策略、审计合一，参数化司法辖区 |
| **跨链孤岛** | 资产困在单链，无法对接外部生态 | `bridge` 对结算资产做门限签名锁定/铸造，IBC 直连 Cosmos |

**目标：让一份实物资产收益权从发行、二级流转、周期分红到跨链结算的全过程，在链上可追溯、可结算、可监管。**

---

## 2. EnergyChain 的定位

一句话：**面向全球能源 / RWA 收益权结算场景的应用专用 L1 公链。**

- **应用专用**：不是通用 L1，所有原生能力都围绕"可信数据 → 合规资产 → 动态做市 → 原子结算 → 跨链"展开。
- **RWA 优先**：参考蚂蚁数科"两链一桥"与 PicWe 链上基建，原生支持收益权代币化、动态地板价、周期分红。
- **EVM 兼容**：基于 Cosmos EVM，Solidity 合约可与原生模块共存。（原生模块的 EVM precompile 在本轮重构中**暂缓**，待 9 模块接口稳定后再补，详见 `chain/genesis.go` 的 `NativePrecompileAddresses`。）
- **可监管**：身份、审计日志、制裁过滤全部内置，监管方可作为只读节点接入。

EnergyChain **不做** 的事：

- 不做通用 DeFi 公链（虽 EVM 兼容，但定位不是这个）。
- 不做内置法币入金（链下 KYC/AML 由各 Issuer 完成，链上只校验合规标志）。
- 不做 PoW（共识层用 CometBFT BFT）。

---

## 3. 整体架构（合并后）

```
       ┌──────────────────────────────────────────────────────────────┐
   5   │  互操作层     bridge（门限签名跨链资产桥）                       │
       ├──────────────────────────────────────────────────────────────┤
   4   │  市场与自动化  market（FBA 撮合）· mincast（动态地板价）          │
       │               offering（IRO 发行）· automation（Cron + 流式支付）│
       ├──────────────────────────────────────────────────────────────┤
   3   │  资产与结算    rwatoken（合规通证）· stableusd（多币种稳定币）     │
       ├──────────────────────────────────────────────────────────────┤
   2   │  数据可信层    assethub（设备 · 计量 · 喂价 · 罚没）              │
       ├──────────────────────────────────────────────────────────────┤
   1   │  合规基座      identity（DID/KYC · 制裁 · 转让策略 · 审计）        │
       ├──────────────────────────────────────────────────────────────┤
   0   │  基础链        Cosmos SDK + Cosmos EVM + IBC + Staking          │
       └──────────────────────────────────────────────────────────────┘
```

阅读顺序建议**自下而上**：下层是基础设施，上层依赖下层。`identity` 是所有合规判断的根；`assethub` 给资产提供可信数据；`stableusd` 是全链统一对价；`rwatoken` 是核心标的；`market / mincast / offering` 提供发行与流动性；`automation` 负责周期任务；`bridge` 把资产带出链外。

InitGenesis / BeginBlock / EndBlock 的固定顺序：`identity → assethub → stableusd → rwatoken → offering → mincast → automation → bridge → market`（`stableusd` 在 `assethub` 之后初始化，因其对账依赖 assethub 的储备喂价；`offering / automation / bridge / market` 运行真实 EndBlocker，其余为 no-op）。`offering` 的 EndBlocker 在募集窗口结束或满额时自动收单（达软顶 → SUCCEEDED，否则 FAILED），并对注资逾期的成功募集自动置为 DEFAULTED。

---

## 4. 模块功能清单（9 个）

每个模块按统一三段式描述：**解决什么 → 关键能力 → 与谁协作**。

### 身份与合规基座

#### 4.1 `identity` —— 合规基座（DID/KYC + 制裁 + 策略 + 审计）

**解决什么**
能源 / RWA 市场上一切动作（发行、持有、转让、赎回）都必须能追到一个有据可查、且满足合规要求的主体。早期蓝图把这件事拆成 `did / policy / sanctions / audit` 四个模块，跨模块调用频繁且边界模糊。现合并为单一合规基座。

**关键能力**
- **账户合规标志（Account）**：每个地址挂 KYC 状态、合格投资人标记、司法辖区、冻结位等，作为上层资产的持有/转让前置判断。
- **注册商（Registrar）**：分级的受信发证方注册表，谁能给主体打合规标志由治理授权。
- **制裁名单（Sanction）**：链上维护被制裁主体，命中即在 `stableusd / rwatoken` 等模块被拒绝。历史命中不可逆删除（审计需要）。
- **转让策略（Policy）**：可组合的转让前置判断式（是否 KYC、是否合格投资人、辖区白/黑名单、是否被制裁），供资产模块统一调用。
- **审计日志（Audit）**：append-only 的治理/敏感动作日志，按模块索引（`AuditByModule`），不可篡改、可监管追查。

**与谁协作**
- 给 `stableusd / rwatoken / mincast / market / offering / bridge` 提供 `ComplianceKeeper` 接口（KYC / 制裁 / 策略校验）：`offering` 在认购时按底层代币的 KYC 要求校验投资人且拒绝制裁地址、出金（收益/退款）拒付制裁地址；`market` 支持按市场可选开启 KYC 与转让策略校验。
- 几乎所有写操作都可向 `identity` 的审计日志留痕。

---

### 数据可信层

#### 4.2 `assethub` —— 数据可信层（设备 + 计量 + 喂价 + 罚没）

**解决什么**
资产价值的根是可信数据：电表读数、设备出力、外部价格、储备金证明。任何单一来源都不可信，且必须能惩罚错报者。早期蓝图把这件事拆成 `device / meter / oracle / dataslash` 四个模块，现合并为统一的"数据可信中枢"。

**关键能力**
- **数据提供者 bond（Provider）**：喂价/计量/公证节点必须独立质押可罚没保证金，与共识 staking 解耦；错报、过期、签名缺失自动罚没。
- **设备注册表（Device）**：每台设备一份链上记录（归属主体、类型、电网区、状态），按运营方索引（`DeviceByOperator`）；上传数据须带设备签名。
- **计量读数（MeteringReading）**：测量周期、起止时间、值（可承诺）、设备签名、质量标记，按设备索引、可批量上链。
- **喂价 Topic 与提交（OracleTopic / OracleSubmission）**：命名 Topic + 多源提交，链上确定性聚合（中位数/均值），供稳定币储备证明、市场指数结算消费。

**与谁协作**
- 给 `stableusd` 提供 `ReserveKeeper`（储备金证明喂价）。
- 给上层资产 / 市场提供可信的计量与价格数据。
- 通过 `bank` 管理 bond，向 `identity` 审计留痕。

---

### 资产与结算层

#### 4.3 `stableusd` —— 多币种合规结算稳定币

**解决什么**
能源 / RWA 结算必须以法币计价的稳定币完成。借鉴 WeUSD 的"全币种统一结算"设计：每个币种独立账本、原生合规、储备可证。

**关键能力**
- **隔离账本**：**不依赖 x/bank**，每个注册稳定币（USD / EUR / ...）有独立的供应、授权图、按账户标志位。供应与权限互不串扰。
- **发行人与配额**：发行人与 `identity` 绑定，多签 mint authority，每发行人有 mint/burn 上限，可治理调整。
- **合规内嵌**：mint / transfer / redeem 全程经 `identity` 做 KYC / 制裁 / 策略校验；支持账户冻结、denom 暂停。
- **储备金证明**：与 `assethub` 喂价联动，按周期核对储备真实性。
- **统一对价**：作为 `rwatoken` 派息、`market` 撮合、`mincast` 做市、`offering` 认购、`bridge` 跨链的统一结算单位。

**与谁协作**
- 被几乎所有上层模块作为 `SettlementKeeper` 调用（余额查询与原子划转）。
- 合规走 `identity`，储备走 `assethub`。

---

#### 4.4 `rwatoken` —— 合规资产通证

**解决什么**
收益权、储能份额、绿色债券、电站股权要上链，但持有人必须 KYC、满足司法辖区限制、支持锁仓/冻结/强制划转——普通 ERC-20 没有这些能力。

**关键能力**
- 兼容 ERC-3643 / ERC-1400 思路的合规通证：转让前置策略由 `identity` 提供。
- **强制操作**：发行人可强制划转、冻结、回收（写审计）。
- **分红快照与派息**：按持仓快照 push 分红，用 `stableusd` 派息；快照逻辑显式排除赎回托管账户（`RedemptionEscrow`），避免把托管余额计入分红。
- **赎回队列**：T+N 排队赎回，支持部分赎回。
- 借鉴蚂蚁数科朗新/协鑫案例的"收益权 mini-IPO + 每两周自动兑现"模式。

**与谁协作**
- 持仓资格 / 制裁过滤走 `identity`；派息与赎回对价走 `stableusd`；初始发行走 `offering`；二级地板价做市走 `mincast`。

---

### 市场与自动化层

#### 4.5 `mincast` —— 动态地板价做市

**解决什么**
PicWe / Origin-Mincast 的核心创新：把运营交易手续费持续注入链上底池，让资产"地板价"随交易频次抬升——交易越频繁、最低收益越高，把静态收益权代币变成动态增值的链上金融产品。

**关键能力**
- **联合曲线（Bonding Curve）做市**：每个资产一个 `Market`（按 denom 索引），按曲线买卖，提供随时可成交的地板价。
- **手续费回灌底池**：成交手续费注入储备池，单调抬升地板价，形成"交易越多、地板越高"的正反馈。
- 合规走 `identity`，对价走 `stableusd`。

**与谁协作**
- 为 `rwatoken` 提供二级流动性与价格发现；被 `automation` 用于周期性做市/结算动作。

---

#### 4.6 `offering` —— 初始 RWA 发行（IRO）

**解决什么**
资产首次上链发行（Initial RWA Offering）需要一套受控的认购、募集、清算流程，类似"链上 mini-IPO"。

**关键能力**
- **发行（Offering）**：定义募集标的、价格、额度、时间窗。
- **认购（Subscription）**：投资人按地址认购，受 `identity` 合规校验。
- 募集对价走 `stableusd`，标的资产由 `rwatoken` 铸造交付（`RWAKeeper`）。

**与谁协作**
- 上游连 `rwatoken`（标的）与 `stableusd`（对价）；合规走 `identity`；发行完成后二级流动性交给 `mincast / market`。

---

#### 4.7 `market` —— 频繁批量拍卖撮合

**解决什么**
结算资产之间（多币种 FX 等）需要一个反抢跑的撮合场所。普通连续订单簿易被夹击；电力/金融出清要求"块边界原子性"。

**关键能力**
- **统一价批量拍卖（Frequent Batch Auction, FBA）**：订单在批次窗口内累积，EndBlock 以单一统一价出清，最大化成交量、结构性消除抢跑。
- **托管模型**：BUY 单按限价托管 quote 资产，SELL 单托管 base 单位到市场受控账户。
- **伸缩式结算（telescoping）**：累计地板法精确分配成交对价，保证每币种守恒（含手续费与取整）。
- **制裁门控**：被制裁主体的挂单被排除出撮合集，其托管保持锁定直至解禁。
- **批次活性**：即使某次结算失败，`LastBatchTime` 仍推进，避免每块忙等。
- 当前 scope 为 `stableusd` 结算 denom（多币种 FX）；RWA 二级流动性交由 `mincast / offering`。

**与谁协作**
- 撮合对价与标的均为 `stableusd` denom；合规与制裁走 `identity`。

---

#### 4.8 `automation` —— 链上 Cron + 流式支付

**解决什么**
"周期性自动动作"（定时结算、定时做市、定时派息）与"连续计费"（充电按 kWh、订阅按秒）如果都靠链下触发，就有单点故障与运维负担。早期蓝图拆成 `scheduler + streampay`，现合并。

**关键能力**
- **链上 Cron（Schedule）**：周期性 msg 自动执行，一次签名→永续触发，可暂停/吊销。
- **流式支付（Stream）**：Sablier / Superfluid 风格的 per-second 连续流，链上累计、提取时结算，支持暂停/终止/转让。
- 可触发 `mincast` 做市、`rwatoken` 派息等下游动作（`MincastKeeper` / `RWAKeeper`）。

**与谁协作**
- 对价走 `stableusd`；合规走 `identity`；下游联动 `mincast / rwatoken`。

---

### 互操作层

#### 4.9 `bridge` —— 跨链结算资产桥

**解决什么**
资产不能困在单链。需要把结算资产（先支持 `stableusd`）安全地锁定/铸造到外部链，并保持 1:1。

**关键能力**
- **外部链注册表（ExternalChain）** 与 **资产映射（Asset）**：定义可桥接的目标链与资产对。
- **门限签名锁定/铸造**：本链锁定 → 外部铸造，外部销毁 → 本链释放，由公证人集合门限签名保证。
- 合规走 `identity`，资产移动走 `stableusd`（`SettlementKeeper`）。

**与谁协作**
- 锁定/释放对象为 `stableusd` 余额；与 IBC 互补（IBC 直连 Cosmos 生态，bridge 面向非 IBC 外部链）。

---

## 5. 十条通用设计原则

| # | 原则 | 落地表现 |
|---|---|---|
| 1 | **合并而非堆叠** | 9 个模块覆盖原 22 模块的全部能力，跨模块调用更少、攻击面更小 |
| 2 | **合规基座单一** | KYC / 制裁 / 策略 / 审计全在 `identity`，上层通过统一接口调用 |
| 3 | **结算单位统一** | `stableusd` 隔离账本是全链唯一对价，避免 x/bank 串扰 |
| 4 | **资产唯一性** | `rwatoken` 强制操作 + 赎回托管隔离，分红快照不计托管余额 |
| 5 | **地板价正反馈** | `mincast` 手续费回灌底池，交易越频繁地板越高（PicWe 模式） |
| 6 | **反抢跑撮合** | `market` 用 FBA 统一价批量出清，结构性消除抢跑 |
| 7 | **数据可罚没** | `assethub` 数据提供者独立 bond，错报罚没，与共识 slashing 解耦 |
| 8 | **算术守恒** | 定点运算用 big.Int 防溢出，结算用伸缩式累计保证每币种守恒 |
| 9 | **可治理参数化** | 限速、配额、费率、司法辖区规则全部可治理升级 |
| 10 | **失败不卡链** | EndBlocker 即便单市场结算失败也推进时钟，避免忙等停链 |

---

## 6. 标准对齐

| 域 | 对齐标准 |
|---|---|
| 身份 | W3C DID Core 1.0、Verifiable Credentials 2.0、Status List 2021 |
| 通证 | ERC-20 / 1400（证券型）/ 3643（合规通证）思路 |
| 稳定币 | 多币种隔离账本（WeUSD 式）、FATF Travel Rule 字段透传 |
| 绿证 / 碳 | EnergyTag、I-REC、EN 16325、Verra / Gold Standard、Article 6.4（后续模块） |
| 风控 | FATF 制裁过滤、IOSCO PFMI（撮合/结算） |
| 跨链 | IBC（Cosmos）、门限签名桥（外部链） |

---

## 7. 分阶段路线图

| 阶段 | 目标 | 涉及模块 | 里程碑 |
|---|---|---|---|
| **M1 · 合规基座 + 数据可信** | 身份/制裁/策略/审计闭环，数据可信中枢上线 | `identity` / `assethub` | 主体合规标志 + 设备签名 + 多源喂价 + bond 罚没打通 |
| **M2 · 资产与结算** | 链上发行可流转的合规资产，统一稳定币结算 | `stableusd` / `rwatoken` | 一笔收益权代币从发行 → 转让 → 分红 → 赎回；稳定币隔离账本可证储备 |
| **M3 · 发行与流动性** | IRO 发行 + 动态地板价 + FBA 撮合 | `offering` / `mincast` / `market` | 第一笔 IRO 募集成功；地板价随交易抬升；FBA 统一价出清跑通 |
| **M4 · 自动化与互联** | 周期任务/流式支付 + 跨链 | `automation` / `bridge` | PPA/分红链上自动结算；流式计费跑通；结算资产门限桥接通 |
| **M5 · EVM precompile（暂缓）** | 把 9 模块暴露为 EVM precompile | 全部 | 接口稳定后开放 Solidity 直调（当前 `NativePrecompileAddresses` 为空） |

每阶段结束都要做：安全审计、经济模型评审（gas / bond / 费率 / 罚没比例）、标准合规自查。

---

## 8. 从 22 模块到 9 模块的合并映射

本轮"激进合并"把早期 22 个细粒度模块的代码全部删除重写，能力归并到 9 个模块：

| 新模块 | 吸收的旧模块 | 说明 |
|---|---|---|
| `identity` | `did` + `policy` + `sanctions` + `audit` | 合规基座：身份/KYC/辖区标志 + 转让策略 DSL + 制裁名单 + append-only 审计 |
| `assethub` | `device` + `meter` + `oracle` + `dataslash` | 数据可信中枢：设备注册 + 计量读数 + 多源喂价 + 可罚没 bond |
| `stableusd` | `stablecoin` | 升级为多币种隔离账本（WeUSD 式），去 x/bank 依赖 |
| `rwatoken` | `rwa` | 合规证券型通证 + 分红快照 + 赎回队列 |
| `mincast` | （新增）| Origin-Mincast 联合曲线动态地板价做市（PicWe 模式） |
| `offering` | （新增）| 初始 RWA 发行（IRO，链上 mini-IPO） |
| `market` | `market` + `auction` + `clearing` | FBA 统一价批量拍卖撮合 + 原子结算（当前 scope：stableusd FX） |
| `automation` | `scheduler` + `streampay` | 链上 Cron + 流式支付合一 |
| `bridge` | （新增，替代旧 EVM 桥精灵）| 结算资产门限签名跨链桥 |

被完全删除、未单独保留的旧模块：

| 旧模块 | 去向 |
|---|---|
| `eac` / `carbon` / `cfe247` | 绿证/碳/24-7 匹配作为后续资产子类型规划，本轮不在 9 模块内（可在 `rwatoken` 之上扩展或后续单列） |
| `contract` | PPA/双边合同的周期结算能力下沉到 `automation`（Cron）+ `stableusd`（结算） |
| `escrow` | 托管能力内嵌到各模块的受控账户（如 `market` 托管、`rwatoken` 赎回托管） |
| `dispute` | 争议/仲裁作为后续合规扩展，暂未在 9 模块内 |

> 链上升级映射见 `chain/upgrades/v1_1_0/upgrade.go`：新增 9 个 store key，删除遗留 `energy` store；`identity` 复用同名 store key（属新增，非删除）。

---

## 9. 名词表

| 术语 | 含义 |
|---|---|
| **RWA** | Real World Asset，真实世界资产（收益权 / 股权 / 债券等）上链通证化 |
| **IRO** | Initial RWA Offering，初始 RWA 发行（链上 mini-IPO，对应 `offering`） |
| **Mincast 地板价** | 把交易手续费持续注入底池、随交易频次单调抬升的资产最低成交价（`mincast`，PicWe/Origin 模式） |
| **FBA** | Frequent Batch Auction，频繁批量拍卖，统一价出清、反抢跑撮合（`market`） |
| **隔离账本** | `stableusd` 每币种独立的供应/授权/标志位，不依赖 x/bank |
| **伸缩式结算** | telescoping，按累计地板法精确分配成交对价、保证每币种守恒 |
| **bond** | 数据提供者 / 公证人必须缴纳的可罚没保证金（`assethub`） |
| **DID** | Decentralized Identifier，W3C 去中心化身份标识（并入 `identity`） |
| **VC** | Verifiable Credential，W3C 可验证凭证 |
| **PoR** | Proof of Reserve，储备金证明（`assethub` 喂给 `stableusd`） |
| **DvP** | Delivery vs Payment，券款对付（资产与对价原子结算） |
| **EAC** | Energy Attribute Certificate，能源属性证书（绿证统称，后续资产子类型） |
| **Article 6.4** | 巴黎协定 6.4 条机制下的国际减排成果转让框架（后续规划） |

---

## 文档维护

- 本文档是 EnergyChain 原生功能的**单一事实源**（single source of truth），开发优先级与排期围绕它展开。
- 任何模块的功能新增、字段调整、删除，先改本文档再开 PR。
- 各模块的技术细节以代码内 doc 注释为准（`chain/x/<module>/`）；升级流程见 `chain/upgrades/`；运维见 `chain/docs/RUNBOOK.md`。
