package file

const fileReadDesc = `读取文件内容。

用法：
- 默认读取前 1000 行，超出截断并提示继续读取方式
- start_line/end_line 读取指定范围，单次上限 2000 行
- 文件超过 10MB 会拒绝读取
- 只能读取文件，不能读目录（用 file_list）`

const fileWriteDesc = `创建新文件或完整覆盖已有文件，自动创建父目录。

用法：
- 修改已有文件优先用 file_edit（只发送变更部分）或 file_patch（diff 批量修改）
- 仅在创建新文件或需完整重写时使用本工具
- 不要创建文档文件（*.md、README）除非用户明确要求`

const fileAppendDesc = `追加内容到文件末尾（文件不存在则创建）。

用法：
- 配合 file_write 分段写入大文件
- 追加模式直接写入，无需读取已有内容`

const fileEditDesc = `精确查找并替换文件内容。

用法：
- 修改已有文件时优先使用本工具，而非 file_write 重写整个文件
- old_string 必须与文件内容完全一致（包括缩进和空白）
- old_string 必须在文件中唯一匹配，多处匹配需包含更多上下文
- new_string 为空则删除匹配文本
- 用最小唯一匹配（2-4 行相邻代码即可），避免大段不必要上下文
- 单文件不超过 10MB`

const grepDesc = `搜索文件或目录中的文本内容。

用法：
- 默认纯文本匹配，use_regex=true 启用正则
- include 按文件名 glob 过滤（如 *.go、*.{js,ts}），支持 ** 模式
- context 显示匹配行前后的上下文行数
- max_count 最大返回结果数，默认 100
- 超过 10MB 的文件自动跳过
- 搜索文件内容用 grep，按文件名查找用 glob`

const fileListDesc = `列出目录下的文件和文件夹。

返回每个条目的名称、类型（文件/目录）和大小。`

const globDesc = `按 glob 模式搜索文件路径，支持 ** 扩展语法，返回匹配列表（最多 100 个）。

用法：
- 支持 **/*.go、src/**/*.{js,ts} 等模式
- 按文件名或路径模式查找，不搜索文件内容（搜索内容用 grep）
- 多轮搜索时，先 glob 定位文件再 grep 搜索内容`

const filePatchDesc = `使用 unified diff 格式批量修改文件，支持模糊匹配（行号允许偏差）。

用法：
- 适用少量精确修改（少于文件 50% 内容且少于 10 个 hunk）
- 超出此范围时，用 file_write + file_append 重写整个文件
- 支持同时修改多个文件

格式：
  --- file
  +++ file
  @@ -start,count +start,count @@
   context line
  -deleted line
  +inserted line
   context line`
