# 人物问答基线：指标与时延测量办法

本文件只记录测量方法。未跑通实现前不填写准确率或耗时数字。

## 样本标识

- 人物：`person-*`
- 单集：`ep-*`
- 片段：`<episode_id>:seg-<n>`
- 问题：`q-*`
- 来源版本：实现写入索引后记录 `source_kind` + `source_version`（产物集 ID 或 Show Notes 摘要），基线 JSON 不硬编码未来表名。

## 分类口径

| 指标 | 分子 | 分母 | 排除 |
| --- | --- | --- | --- |
| 身份准确率 | 候选项与标注人物 ID 一致 | 有标注参与者的出场 | 仅提及姓名 |
| 发言归属准确率 | 片段归属 person_id 与标注一致 | 标注为 confirmed 的片段 | pending、混标签未纠正 |
| 发言归属覆盖率 | 已确认归属片段数 | 该人在本集全部片段 | pending 计入分母但不计入人物观点 |
| 关键证据召回 | 命中 `expected_fragment_ids` | 有直接证据的问题 | 无答案题 |
| 无依据归属/推演 | 把 pending、他人转述、失效来源或未标识推演当作本人原话的次数 | 固定验收集 | — |
| 弃答 | `cannot_judge` / `not_a_participant` 且符合预期 | 无答案题 | 有证据题弃答算失败，不能靠全部弃答通过 |

归属与内容相关分开记录：命中文本提到某人，不等于该片段属于该人。

## 时延

对每次提问记录：

1. 首个可见状态时间（进度/准备/错误，以用户能读到的第一条状态为准）
2. 有效答案时间（出现可核对正文，含模拟标识；不是状态事件时间）
3. 是否发生网页补搜、检索轮数、入选来源数

预算在实现可运行后按样本校准，本票不承诺未测得的阈值。

## 结果记录格式

后续票按问题一行记录，字段固定为：

```text
question_id | expected_kind | identity_ok | attribution_ok | recalled_fragments | web_search | inference_marked | first_status_ms | usable_answer_ms | notes
```

未运行的功能写 `not_run`，不得填写伪造通过。
