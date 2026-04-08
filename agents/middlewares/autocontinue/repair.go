package autocontinue

// repairJSON 尝试修复被截断的 JSON 字符串。
// 通过逐字符解析追踪 JSON 结构状态，遇到 EOF 时自动闭合所有未关闭的结构。
// 如果输入为空或无法修复则返回原字符串。
func repairJSON(s string) string {
	if len(s) == 0 {
		return s
	}

	type stackItem struct {
		kind byte // '{', '[', '"'
	}

	var stack []stackItem
	i := 0
	n := len(s)
	inString := false
	escaped := false

	for i < n {
		ch := s[i]

		if escaped {
			escaped = false
			i++
			continue
		}

		if inString {
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
				if len(stack) > 0 && stack[len(stack)-1].kind == '"' {
					stack = stack[:len(stack)-1]
				}
			}
			i++
			continue
		}

		switch ch {
		case '"':
			inString = true
			stack = append(stack, stackItem{kind: '"'})
		case '{':
			stack = append(stack, stackItem{kind: '{'})
		case '}':
			if len(stack) > 0 && stack[len(stack)-1].kind == '{' {
				stack = stack[:len(stack)-1]
			}
		case '[':
			stack = append(stack, stackItem{kind: '['})
		case ']':
			if len(stack) > 0 && stack[len(stack)-1].kind == '[' {
				stack = stack[:len(stack)-1]
			}
		}
		i++
	}

	if len(stack) == 0 {
		return s
	}

	// 从栈顶向下闭合所有未关闭的结构
	result := []byte(s)

	// 如果在转义状态中截断（如 content 以 \ 结尾），移除尾部的孤立反斜杠
	if escaped && len(result) > 0 && result[len(result)-1] == '\\' {
		result = result[:len(result)-1]
	}

	for j := len(stack) - 1; j >= 0; j-- {
		switch stack[j].kind {
		case '"':
			result = append(result, '"')
		case '{':
			// 检查是否以逗号或冒号结尾需要清理
			result = trimTrailingJSONGarbage(result)
			result = append(result, '}')
		case '[':
			result = trimTrailingJSONGarbage(result)
			result = append(result, ']')
		}
	}

	return string(result)
}

// trimTrailingJSONGarbage 移除尾部不完整的 JSON 片段（逗号、冒号、不完整的 key）
func trimTrailingJSONGarbage(data []byte) []byte {
	for len(data) > 0 {
		ch := data[len(data)-1]
		switch ch {
		case ',', ':', ' ', '\t', '\n', '\r':
			data = data[:len(data)-1]
			continue
		}
		break
	}
	// 如果尾部是一个孤立的带引号字符串（未配对的 key），需要移除它
	// 例如 {"filepath":"/file.go","cont" → 移除 ,"cont" 部分
	if len(data) > 0 && data[len(data)-1] == '"' {
		// 向前找到这个字符串的起始引号
		end := len(data) - 1
		pos := end - 1
		for pos >= 0 {
			if data[pos] == '"' && (pos == 0 || data[pos-1] != '\\') {
				break
			}
			pos--
		}
		if pos >= 0 {
			// 检查引号前面是 , 还是 { — 这表示它是一个孤立的 key
			before := pos - 1
			for before >= 0 && (data[before] == ' ' || data[before] == '\t' || data[before] == '\n' || data[before] == '\r') {
				before--
			}
			if before >= 0 && (data[before] == ',' || data[before] == '{' || data[before] == '[') {
				if data[before] == ',' {
					data = data[:before]
				} else {
					// { 或 [ 开头后面紧跟的 key，保留 { 或 [
					data = data[:before+1]
				}
			}
		}
	}
	return data
}
