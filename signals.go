package spam

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/git-pkgs/scan"
)

const (
	minPromotions    = 3
	minOffers        = 2
	minPhraseRepeats = 6
	phraseWords      = 4
	minURLRepeats    = 3
	maxCueGap        = 256
	minLinks         = 10
	linkPercent      = 35
	percentScale     = 100
	minLineBytes     = 24
	minLineRepeats   = 3
)

type occurrence struct {
	start int
	end   int
	count int
}

type link struct {
	start       int
	end         int
	destination string
}

type analysis struct {
	evidence     map[string]Evidence
	measurements []Measurement
}

type textMatches struct {
	promotions []scan.Match
	offers     []scan.Match
	links      []link
	custom     map[string]scan.Match
}

func (s *Scanner) inspect(doc Document, scratch *scan.Scratch) (analysis, error) {
	found := make(map[string]Evidence)
	matches, err := s.matchText(doc.Data, scratch)
	if err != nil {
		return analysis{}, err
	}
	measurements := make([]Measurement, 0, len(s.rules))
	for _, rule := range s.rules {
		id := rule.ID
		item := Measurement{Path: doc.Path, Rule: id, TextBytes: len(doc.Data)}
		switch id {
		case linkHeavyRule:
			item.Count = len(matches.links)
			item.URLBytes = addLinkDensityEvidence(doc, matches.links, found)
		case keywordRule:
			rawPhrase, phrase := repeatedPhrase(doc.Data)
			if phrase.count >= minPhraseRepeats {
				found[id] = newEvidence(doc, phrase.start, phrase.end, phrase.count, "repeated four-word sequence separated by spaces")
			}
			item.Count = rawPhrase.count
			item.TextCount = phrase.count
		case promotionalRule:
			addPromotionEvidence(doc, matches, found)
			item.Count = len(matches.promotions)
			item.OfferCount = len(matches.offers)
		case repeatedURLRule:
			item.Count, item.ContextCount = addRepeatedURLEvidence(doc, matches, found)
		case repeatedLineRule:
			line := repeatedLine(doc.Data)
			if line.count >= minLineRepeats {
				found[id] = newEvidence(doc, line.start, line.end, line.count, "repeated nonempty line")
			}
			item.Count = line.count
		default:
			if match, exists := matches.custom[id]; exists {
				item.Count = 1
				found[id] = newEvidence(doc, int(match.From), int(match.To), 1, rule.Description)
			}
		}
		measurements = append(measurements, item)
	}
	return analysis{evidence: found, measurements: measurements}, nil
}

func (s *Scanner) matchText(data []byte, scratch *scan.Scratch) (textMatches, error) {
	matches := textMatches{custom: make(map[string]scan.Match)}
	if s.db == nil {
		return matches, nil
	}
	urlEnd := 0
	err := s.db.Scan(data, scratch, func(m scan.Match) error {
		switch m.ID {
		case promotionPattern:
			matches.promotions = append(matches.promotions, m)
		case offerPattern:
			matches.offers = append(matches.offers, m)
		case urlPattern:
			if int(m.From) < urlEnd {
				break
			}
			item, ok := readLink(data, int(m.From))
			urlEnd = item.end
			if ok {
				matches.links = append(matches.links, item)
			}
		default:
			if rule, exists := s.custom[m.ID]; exists {
				matches.custom[rule.ID] = m
			}
		}
		return nil
	})
	return matches, err
}

func addPromotionEvidence(doc Document, matches textMatches, found map[string]Evidence) {
	if len(matches.promotions) >= minPromotions {
		first := matches.promotions[0]
		found[promotionalRule] = newEvidence(doc, int(first.From), int(first.To), len(matches.promotions), "calls to action or promotional claims")
	} else if len(matches.offers) >= minOffers && len(matches.promotions) > 0 && len(matches.links) > 0 {
		first := matches.offers[0]
		detail := fmt.Sprintf("%d offer claims with %d promotional cues and %d URLs", len(matches.offers), len(matches.promotions), len(matches.links))
		found[promotionalRule] = newEvidence(doc, int(first.From), int(first.To), len(matches.offers), detail)
	}
}

func readLink(data []byte, start int) (link, bool) {
	end := len(data)
	for offset := start; offset < len(data); {
		r, size := utf8.DecodeRune(data[offset:])
		if unicode.IsSpace(r) || strings.ContainsRune("\"'<>`[]{}()", r) {
			end = offset
			break
		}
		offset += size
	}
	raw := string(bytes.TrimRight(data[start:end], ".,;:!?"))
	item := link{start: start, end: start + len(raw)}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return item, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	item.destination = u.String()
	return item, true
}

func addRepeatedURLEvidence(doc Document, matches textMatches, found map[string]Evidence) (int, int) {
	counts := make(map[string]occurrence)
	var best occurrence
	for _, item := range matches.links {
		best = recordOccurrence(counts, item.destination, item.start, item.end, best)
	}
	var contextual occurrence
	var cue scan.Match
	next := 0
	for _, promotion := range matches.promotions {
		for next < len(matches.links) && matches.links[next].end <= int(promotion.From) {
			next++
		}
		item, ok := nearestCueLink(doc.Data, matches.links, next, promotion)
		if !ok || counts[item.destination].count <= contextual.count {
			continue
		}
		contextual = occurrence{start: item.start, end: item.end, count: counts[item.destination].count}
		cue = promotion
	}
	if contextual.count >= minURLRepeats {
		detail := fmt.Sprintf("same URL after removing fragments; nearest URL to promotional cue at bytes %d:%d, within %d bytes on the same line", cue.From, cue.To, maxCueGap)
		found[repeatedURLRule] = newEvidence(doc, contextual.start, contextual.end, contextual.count, detail)
	}
	return best.count, contextual.count
}

func nearestCueLink(data []byte, links []link, next int, cue scan.Match) (link, bool) {
	start, end := int(cue.From), int(cue.To)
	if next < len(links) && links[next].start < end {
		return link{}, false
	}
	var nearest link
	bestGap := maxCueGap + 1
	for i := max(0, next-1); i < min(len(links), next+1); i++ {
		item := links[i]
		gapStart, gapEnd := item.end, start
		if i == next {
			gapStart, gapEnd = end, item.start
		}
		if gapEnd-gapStart >= bestGap || bytes.ContainsAny(data[gapStart:gapEnd], "\r\n") {
			continue
		}
		nearest, bestGap = item, gapEnd-gapStart
	}
	return nearest, nearest.destination != ""
}

func addLinkDensityEvidence(doc Document, links []link, found map[string]Evidence) int {
	totalBytes := 0
	for _, item := range links {
		totalBytes += item.end - item.start
	}
	if len(links) >= minLinks && totalBytes*percentScale >= len(doc.Data)*linkPercent {
		first := links[0]
		detail := fmt.Sprintf("%d of %d raw bytes are URLs", totalBytes, len(doc.Data))
		found[linkHeavyRule] = newEvidence(doc, first.start, first.end, len(links), detail)
	}
	return totalBytes
}

func repeatedPhrase(data []byte) (occurrence, occurrence) {
	tokens := wordPositions(data)
	var words [phraseWords][]byte
	counts := make(map[string]occurrence)
	textCounts := make(map[string]occurrence)
	var best occurrence
	var textBest occurrence
	var keyBytes []byte
	for i, token := range tokens {
		word := words[0][:0]
		copy(words[:], words[1:])
		wordData := data[token.start:token.end]
		for len(wordData) > 0 {
			r, size := utf8.DecodeRune(wordData)
			word = utf8.AppendRune(word, unicode.ToLower(r))
			wordData = wordData[size:]
		}
		words[phraseWords-1] = word
		if i+1 < phraseWords {
			continue
		}
		keyBytes = keyBytes[:0]
		for j, word := range words {
			if j > 0 {
				keyBytes = append(keyBytes, ' ')
			}
			keyBytes = append(keyBytes, word...)
		}
		key := string(keyBytes)
		start, end := tokens[i+1-phraseWords].start, token.end
		best = recordOccurrence(counts, key, start, end, best)
		if isTextPhrase(data[start:end]) {
			textBest = recordOccurrence(textCounts, key, start, end, textBest)
		}
	}
	return best, textBest
}

func isTextPhrase(data []byte) bool {
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if !unicode.IsLetter(r) && !unicode.IsMark(r) && r != ' ' && r != '\t' {
			return false
		}
		data = data[size:]
	}
	return true
}

func wordPositions(data []byte) []occurrence {
	var tokens []occurrence
	start := -1
	for offset := 0; offset < len(data); {
		r, size := utf8.DecodeRune(data[offset:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || (start >= 0 && unicode.IsMark(r)) {
			if start < 0 {
				start = offset
			}
		} else if start >= 0 {
			tokens = append(tokens, occurrence{start: start, end: offset})
			start = -1
		}
		offset += size
	}
	if start >= 0 {
		tokens = append(tokens, occurrence{start: start, end: len(data)})
	}
	return tokens
}

func repeatedLine(data []byte) occurrence {
	counts := make(map[string]occurrence)
	var best occurrence
	offset := 0
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) >= minLineBytes {
			start := offset + bytes.Index(line, trimmed)
			best = recordOccurrence(counts, strings.ToLower(string(trimmed)), start, start+len(trimmed), best)
		}
		offset += len(line) + 1
	}
	return best
}

func recordOccurrence(counts map[string]occurrence, key string, start, end int, best occurrence) occurrence {
	current := counts[key]
	if current.count == 0 {
		current.start, current.end = start, end
	}
	current.count++
	counts[key] = current
	if current.count > best.count || (current.count == best.count && current.start < best.start) {
		return current
	}
	return best
}
