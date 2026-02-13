package mdiff

// OpKind 表示编辑操作类型
type OpKind int

const (
	// OpEqual 表示行未变化
	OpEqual OpKind = iota
	// OpInsert 表示新增行
	OpInsert
	// OpDelete 表示删除行
	OpDelete
)

// Edit 表示一个编辑操作
type Edit struct {
	Kind OpKind
	// OldPos 操作在旧文本中的行号（从0开始），仅 Equal/Delete 有效
	OldPos int
	// NewPos 操作在新文本中的行号（从0开始），仅 Equal/Insert 有效
	NewPos int
	// Text 行内容（不含换行符）
	Text string
}

// Diff 使用 Myers 算法计算两组行的最小编辑序列
func Diff(oldLines, newLines []string) []Edit {
	n := len(oldLines)
	m := len(newLines)

	if n == 0 && m == 0 {
		return nil
	}

	if n == 0 {
		edits := make([]Edit, m)
		for i, line := range newLines {
			edits[i] = Edit{Kind: OpInsert, NewPos: i, Text: line}
		}
		return edits
	}

	if m == 0 {
		edits := make([]Edit, n)
		for i, line := range oldLines {
			edits[i] = Edit{Kind: OpDelete, OldPos: i, Text: line}
		}
		return edits
	}

	// 优化：跳过公共前缀和后缀，减少 Myers 搜索空间
	prefixLen := 0
	minLen := n
	if m < minLen {
		minLen = m
	}
	for prefixLen < minLen && oldLines[prefixLen] == newLines[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	for suffixLen < minLen-prefixLen && oldLines[n-1-suffixLen] == newLines[m-1-suffixLen] {
		suffixLen++
	}

	// 提取需要 diff 的中间部分
	oldMid := oldLines[prefixLen : n-suffixLen]
	newMid := newLines[prefixLen : m-suffixLen]

	// 对中间部分执行 Myers diff
	var midEdits []Edit
	if len(oldMid) == 0 && len(newMid) == 0 {
		// 完全相同，无需处理
	} else if len(oldMid) == 0 {
		for i, line := range newMid {
			midEdits = append(midEdits, Edit{Kind: OpInsert, NewPos: prefixLen + i, Text: line})
		}
	} else if len(newMid) == 0 {
		for i, line := range oldMid {
			midEdits = append(midEdits, Edit{Kind: OpDelete, OldPos: prefixLen + i, Text: line})
		}
	} else {
		midEdits = myersDiff(oldMid, newMid)
		// 修正偏移量
		for i := range midEdits {
			midEdits[i].OldPos += prefixLen
			midEdits[i].NewPos += prefixLen
		}
	}

	// 组装完整结果：前缀 + 中间 diff + 后缀
	var edits []Edit
	for i := 0; i < prefixLen; i++ {
		edits = append(edits, Edit{Kind: OpEqual, OldPos: i, NewPos: i, Text: oldLines[i]})
	}
	edits = append(edits, midEdits...)
	for i := 0; i < suffixLen; i++ {
		edits = append(edits, Edit{
			Kind:   OpEqual,
			OldPos: n - suffixLen + i,
			NewPos: m - suffixLen + i,
			Text:   oldLines[n-suffixLen+i],
		})
	}

	return edits
}

// myersDiff 实现 Myers 差分算法
func myersDiff(oldLines, newLines []string) []Edit {
	n := len(oldLines)
	m := len(newLines)
	max := n + m

	vSize := 2*max + 1
	v := make([]int, vSize)
	// 预分配 trace 空间，避免反复分配
	trace := make([][]int, 0, min(max+1, 1024))
	offset := max

	for d := 0; d <= max; d++ {
		// 只 copy 实际使用的范围 [offset-d, offset+d]
		snapshot := make([]int, vSize)
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+offset] < v[k+1+offset]) {
				x = v[k+1+offset]
			} else {
				x = v[k-1+offset] + 1
			}
			y := x - k

			for x < n && y < m && oldLines[x] == newLines[y] {
				x++
				y++
			}

			v[k+offset] = x

			if x >= n && y >= m {
				return backtrack(trace, oldLines, newLines, d, offset)
			}
		}
	}

	return nil
}

// backtrack 从 trace 中回溯出编辑序列
func backtrack(trace [][]int, oldLines, newLines []string, d, offset int) []Edit {
	n := len(oldLines)
	m := len(newLines)

	var edits []Edit
	x := n
	y := m

	for d > 0 {
		v := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[k-1+offset] < v[k+1+offset]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := v[prevK+offset]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, Edit{Kind: OpEqual, OldPos: x, NewPos: y, Text: oldLines[x]})
		}

		if x == prevX {
			y--
			edits = append(edits, Edit{Kind: OpInsert, NewPos: y, Text: newLines[y]})
		} else {
			x--
			edits = append(edits, Edit{Kind: OpDelete, OldPos: x, Text: oldLines[x]})
		}

		d--
	}

	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, Edit{Kind: OpEqual, OldPos: x, NewPos: y, Text: oldLines[x]})
	}

	reverseEdits(edits)
	return edits
}

// reverseEdits 原地反转编辑序列
func reverseEdits(edits []Edit) {
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
}
