package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

type SearchResult struct {
	Entry *MemoryEntry
	Score float64
}

type bm25Doc struct {
	entryID string
	tf      map[string]float64
	docLen  float64
}

// BM25 索引
type BM25 struct {
	docs      []bm25Doc
	avgDocLen float64
	idf       map[string]float64
	totalDocs int
}

func tokenize(entry *MemoryEntry) []string {
	var tokens []string
	for _, tag := range entry.Tags {
		tokens = append(tokens, strings.ToLower(tag))
	}
	tokens = append(tokens, textTokenize(entry.Summary)...)
	tokens = append(tokens, textTokenize(entry.Detail)...)
	return tokens
}

func queryTokenize(query string) []string { return textTokenize(query) }

func textTokenize(text string) []string {
	var tokens []string
	var wordBuf strings.Builder
	var hanBuf []rune

	flushWord := func() {
		if wordBuf.Len() > 0 {
			tokens = append(tokens, wordBuf.String())
			wordBuf.Reset()
		}
	}
	flushHanBigrams := func() {
		if len(hanBuf) >= 2 {
			for i := 0; i < len(hanBuf)-1; i++ {
				tokens = append(tokens, string(hanBuf[i])+string(hanBuf[i+1]))
			}
		} else if len(hanBuf) == 1 {
			tokens = append(tokens, string(hanBuf[0]))
		}
		hanBuf = hanBuf[:0]
	}

	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flushWord()
			hanBuf = append(hanBuf, r)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			flushHanBigrams()
			wordBuf.WriteRune(r)
		} else {
			flushHanBigrams()
			flushWord()
		}
	}
	flushHanBigrams()
	flushWord()
	return tokens
}

func (b *BM25) Build(entries []MemoryEntry) {
	b.totalDocs = len(entries)
	b.docs = make([]bm25Doc, 0, len(entries))
	df := make(map[string]int)
	var totalLen float64
	for i := range entries {
		tokens := tokenize(&entries[i])
		tf := make(map[string]float64)
		for _, t := range tokens {
			tf[t]++
		}
		docLen := float64(len(tokens))
		totalLen += docLen
		b.docs = append(b.docs, bm25Doc{entryID: entries[i].ID, tf: tf, docLen: docLen})
		seen := make(map[string]bool)
		for _, t := range tokens {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}
	if b.totalDocs > 0 {
		b.avgDocLen = totalLen / float64(b.totalDocs)
	}
	b.idf = make(map[string]float64, len(df))
	for term, n := range df {
		b.idf[term] = math.Log(1 + (float64(b.totalDocs)-float64(n)+0.5)/(float64(n)+0.5))
	}
}

func (b *BM25) Search(query string, entries []MemoryEntry, topK int) []SearchResult {
	queryTokens := queryTokenize(query)
	if len(queryTokens) == 0 || len(b.docs) == 0 {
		return nil
	}
	entryMap := make(map[string]*MemoryEntry, len(entries))
	for i := range entries {
		entryMap[entries[i].ID] = &entries[i]
	}
	var results []SearchResult
	for _, doc := range b.docs {
		score := 0.0
		for _, qt := range queryTokens {
			tfVal := doc.tf[qt]
			idfVal := b.idf[qt]
			score += idfVal * tfVal * (bm25K1 + 1) / (tfVal + bm25K1*(1-bm25B+bm25B*doc.docLen/b.avgDocLen))
		}
		if score > 0 {
			if entry, ok := entryMap[doc.entryID]; ok {
				results = append(results, SearchResult{Entry: entry, Score: score})
			}
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}
