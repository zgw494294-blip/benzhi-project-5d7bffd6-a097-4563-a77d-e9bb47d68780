# 井澈地下水采样质量放行服务

井澈面向地下水监测团队，把监测批次建立、井位与样品登记、质量规则检查、异常整改复核、技术批准、数据冻结和放行凭据签发串成一条可追溯流程。服务提供版本化 JSON HTTP API，使用本地 SQLite 保存业务记录；每次状态变化都进入摘要相连的只追加审计时间线。

## 构建与运行

需要 Go 1.22 或更高版本。标准构建命令：

```text
go build ./cmd/server
```

开发运行命令：

```text
go run ./cmd/server -addr=127.0.0.1:19081 -db=groundwater-release.db
```

默认监听地址为 `127.0.0.1:19081`，不会绑定通配地址。可以用 `-addr=127.0.0.1:<port>` 指定回环端口，也可以设置 `PORT`；例如 `PORT=19120` 会监听 `127.0.0.1:19120`。显式 `-addr` 优先于 `PORT`。服务拒绝空主机、通配地址、非回环地址及无效端口。

SQLite 启用外键和 WAL。默认数据库文件是 `groundwater-release.db`，可以通过 `-db` 指定其他路径。

## 主要 API

所有写请求都在 JSON 中携带 `idempotencyKey`、`expectedVersion`、`actor` 和 `role`。创建批次时 `expectedVersion` 可为 `0`；后续写请求必须使用上一响应的 `version`。相同 `idempotencyKey` 的同一操作会返回首次提交结果，不会重复推进状态。

- `GET /healthz`：健康检查。
- `POST /api/v1/campaigns`：创建监测批次。
- `POST /api/v1/campaigns/{campaignID}/wells`：登记监测井和采样计划。
- `POST /api/v1/campaigns/{campaignID}/wells:batch`：原子批量登记最多 100 口监测井；失败时返回带 `index` 与 `field` 的完整错误清单。
- `POST /api/v1/campaigns/{campaignID}/samples`：登记样品、现场测量、保存期限和交接链。
- `POST /api/v1/campaigns/{campaignID}/checks:reopen`：审核员在批准前退回已检查批次并使当前检查失效。
- `PATCH /api/v1/campaigns/{campaignID}/samples/{sampleID}`：按样品 `revision` 修订测量、保存事实或追加交接段。
- `POST /api/v1/campaigns/{campaignID}/checks`：执行 `GW-QC-1` 质量规则，只追加检查历史并选择新的当前检查。
- `GET /api/v1/campaigns/{campaignID}/checks`：分页查询检查历史；同时传 `fromCheckId` 和 `toCheckId` 可查询新增、消失及持续失败项。
- `POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/evidence`：追加独立 revision 的补采或书面说明证据。
- `POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/evidence/{revision}:withdraw`：提交人在审核前撤回最新待审证据。
- `POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/review`：审核指定的 `evidenceRevision`，接受或驳回只影响该版本。
- `GET /api/v1/campaigns/{campaignID}/approval-readiness?actor=<批准人>`：只读返回检查新鲜度、未关闭异常、待审证据和职责分离阻断项。
- `POST /api/v1/campaigns/{campaignID}/approve`：携带当前 `checkDigest` 完成技术批准，并记录批准依据。
- `POST /api/v1/campaigns/{campaignID}/freeze`：原子冻结发布数据集并计算摘要。
- `POST /api/v1/campaigns/{campaignID}/credentials`：签发递增序号的不可变放行凭据。
- `GET /api/v1/campaigns/{campaignID}/credentials/verification`：从序号一到目标凭据核验全局凭据链、冻结摘要、凭据摘要和目标批次审计链。
- `GET /api/v1/campaigns/{campaignID}`：查询批次全量状态、未决项、时间线、冻结数据和凭据完整性。

角色标识分别为 `FIELD_LEAD`、`LAB_RECEIVER`、`QUALITY_REVIEWER`、`TECHNICAL_APPROVER` 和 `RELEASE_OFFICER`。质量规则检查监测井完整性、现场空白样、现场平行样、保存时限及交接连续性。井位或样品变化会提高事实修订号并使旧检查失效；整改和审核只提高乐观锁版本。检查执行人以及任何已接受整改版本的审核人不能担任该批次技术批准人。

## 测试与自检

运行全部测试：

```text
go test ./...
```

运行有界端到端自检：

```text
go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=15s
```

自检会创建临时 SQLite 数据库，在指定回环地址真实启动 HTTP 服务，依次完成批次、井位、常规样和平行样登记，触发并关闭空白样异常，再执行批准、冻结、签发和完整性查询。无论成功或失败，服务都会在时限内关闭并清理临时数据库。
