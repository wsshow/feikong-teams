package file

const fileReadDesc = `读取文件内容。

用法：
- 默认读取文件前 200 行，超出部分截断并提示总行数
- 可通过 start_line/end_line 参数读取指定行范围
- 结果包含行号，便于后续 file_edit 定位
- 只能读取文件，不能读取目录（用 file_list 列目录）`

const fileWriteDesc = `创建新文件或完整覆盖已有文件，自动创建父目录。

用法：
- 修改已有文件优先使用 file_edit（只发送变更部分）或 file_patch（diff 格式批量修改）
- 仅在创建新文件或需要完整重写时使用本工具
- 不要创建文档文件（*.md、README）除非用户明确要求

## 重要：分段写入规则
content 单次不要超过 200 行。超过 200 行的内容会因为输出 token 限制导致 JSON 被截断而调用失败。

大文件写入流程：
1. file_write 写入前半部分（不超过 200 行）
2. file_append 追加剩余内容（每段不超过 200 行）
3. 重复 file_append 直到全部写完

示例 - 创建 500 行文件：
  第1步: file_write(filepath="index.html", content="前200行...")
  第2步: file_append(filepath="index.html", content="第201-400行...")
  第3步: file_append(filepath="index.html", content="第401-500行...")`

const fileAppendDesc = `追加内容到文件末尾（文件不存在则创建）。

用法：
- 配合 file_write 分段写入大文件，每次追加不超过 200 行
- 创建超过 200 行的文件时，先 file_write 写前半部分，再 file_append 逐段追加`

const fileEditDesc = `精确查找并替换文件内容。

用法：
- 修改已有文件时优先使用本工具，而非 file_write 重写整个文件
- old_string 必须与文件中的内容完全一致（包括缩进和空白）
- old_string 必须在文件中唯一匹配，如有多处匹配需包含更多上下文
- new_string 为空则删除匹配文本
- 使用最小唯一匹配（通常 2-4 行相邻代码即可），避免包含大段不必要的上下文`

const grepDesc = `搜索文件或目录中的文本内容。

用法：
- 默认纯文本匹配，设置 use_regex=true 启用正则
- 用 include 参数按文件名 glob 过滤（如 *.go、*.{js,ts}）
- 用 context 参数显示匹配行前后的上下文
- 搜索文件内容用 grep，按文件名查找用 glob`

const fileListDesc = `列出目录下的文件和文件夹。

返回每个条目的名称、类型（文件/目录）和大小。`

const globDesc = `按 glob 模式快速搜索文件路径，返回匹配的文件列表（最多 100 个）。

用法：
- 支持 **/*.go、src/**/*.{js,ts} 等模式
- 用于按文件名或路径模式查找文件，不搜索文件内容（搜索内容用 grep）
- 需要多轮搜索时，先 glob 找文件再 grep 搜内容`

const filePatchDesc = `使用 unified diff 格式批量修改文件，支持模糊匹配（行号允许偏差）。

用法：
- 适用于对已有文件做少量精确修改（少于文件 50% 内容且少于 10 个 hunk）
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
