# 和合本简体经文数据

供背经的卷、章、节选择器使用。按卷拆分为 `1.json` 至 `66.json`，按需加载；每卷为章数组，每章为按节号排列的原文数组。数据共 66 卷、1189 章、31103 节，加载时不依赖第三方接口。原始数据不清除注释，填入背经原文时才按产品规则清除含“原文”的小括号注释。

来源：[k-mktr/chinese_union_simplified_zh](https://huggingface.co/datasets/k-mktr/chinese_union_simplified_zh)，固定版本 `791fcfef62b51a6356682bcc4c8aa6bb60ea6c63`，`data/data.parquet`。

来源数据集声明：1919 年和合本，公有领域（Public domain）。来源 SHA-256：`2bf28cbeb4449f76c44ed59d3072808543209b4e76399d67d0764874b35a44e8`。

转换时按 `book_number`、`chapter`、`verse_number` 定位，验证卷、章、节连续性后保留原文。同一节号的后缀片段合并，不根据数组原有行次猜测节号。
