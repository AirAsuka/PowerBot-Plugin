# 谁是卧底词库

插件首次使用词库时会创建 `data/undercover/words.db`，并将内置词对按分类写入 SQLite。之后每次升级只补充缺少的内置词，不覆盖管理员维护的状态和统计。

## 表结构

```sql
CREATE TABLE undercover_words (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    word_a      TEXT    NOT NULL,
    word_b      TEXT    NOT NULL,
    pair_key    TEXT    NOT NULL UNIQUE,
    category    TEXT    NOT NULL DEFAULT '通用',
    difficulty  INTEGER NOT NULL DEFAULT 2 CHECK (difficulty BETWEEN 1 AND 3),
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    use_count   INTEGER NOT NULL DEFAULT 0,
    created_by  INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL
);
```

- `word_a`、`word_b`：相近词对。开局时随机决定哪一个是平民词，避免形成固定答案。
- `pair_key`：将两个词转成小写、排序后生成的唯一键，阻止反向重复，例如“牛奶/豆浆”和“豆浆/牛奶”只能存在一条。
- `category`：词条分类，便于后续扩展分类房间。
- `difficulty`：1 简单、2 普通、3 困难。
- `enabled`：软开关；禁用词条不会被抽到，也不会丢失审计信息。
- `use_count`：词条被随机抽取的次数。
- `created_by`：添加词条的管理员 QQ；内置词为 0。
- `created_at`：Unix 秒级创建时间。

索引包括 `(enabled, difficulty, category)` 和 `use_count`。前者服务抽词筛选，后者可用于后续实现低频词优先。

## 管理指令

仅群管理员、群主或机器人超级用户可以使用：

```text
添加卧底词 牛奶|豆浆|饮品|1
启用卧底词 12
禁用卧底词 12
卧底词库
卧底词库 2
卧底词库统计
```

