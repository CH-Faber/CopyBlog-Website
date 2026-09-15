package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Object is the minimum S3 metadata needed to resolve and version an image.
// ETag is included in the public filename so replacing an S3 object produces a
// new website URL instead of leaving browsers with stale cached bytes.
type Object struct {
	Key  string `json:"sourceKey"`
	ETag string `json:"etag"`
}

type PublishedAsset struct {
	SourceKey string `json:"sourceKey"`
	ETag      string `json:"etag"`
	PublicURL string `json:"publicUrl"`
}

type Index struct {
	objects []Object
}

var (
	wikiImagePattern     = regexp.MustCompile(`!\[\[([^\]\r\n]+)\]\]`)
	markdownImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)`)
)

var imageExtensions = map[string]struct{}{
	".avif": {},
	".gif":  {},
	".jpeg": {},
	".jpg":  {},
	".png":  {},
	".webp": {},
}

func IsImageKey(key string) bool {
	_, ok := imageExtensions[strings.ToLower(path.Ext(strings.TrimSpace(key)))]
	return ok
}

func NewIndex(objects []Object) *Index {
	filtered := make([]Object, 0, len(objects))
	for _, object := range objects {
		object.Key = cleanObjectKey(object.Key)
		object.ETag = strings.Trim(strings.TrimSpace(object.ETag), `"`)
		if object.Key == "" || !IsImageKey(object.Key) {
			continue
		}
		filtered = append(filtered, object)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Key < filtered[j].Key })
	return &Index{objects: filtered}
}

func cleanObjectKey(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	value = strings.TrimPrefix(value, "./")
	return strings.TrimPrefix(value, "/")
}

func cleanTarget(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "<>")
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ReplaceAll(value, `\`, "/")
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	if index := strings.Index(value, "#"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func isRemoteOrAbsolute(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "blob:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "#")
}

func publicURL(object Object) string {
	extension := strings.ToLower(path.Ext(object.Key))
	sum := sha256.Sum256([]byte(object.Key + "\x00" + object.ETag))
	digest := hex.EncodeToString(sum[:])
	return "/obsidian-assets/" + digest[:2] + "/" + digest + extension
}

func (index *Index) Resolve(rawTarget, articleKey string) (PublishedAsset, error) {
	target := cleanTarget(rawTarget)
	if target == "" || !IsImageKey(target) {
		return PublishedAsset{}, fmt.Errorf("不是受支持的本地图片: %s", rawTarget)
	}
	if strings.Contains(target, "\x00") || target == ".." || strings.HasPrefix(target, "../") || strings.Contains(target, "/../") {
		return PublishedAsset{}, fmt.Errorf("图片路径越过了 Obsidian 仓库边界: %s", rawTarget)
	}
	if len(target) >= 3 && target[1] == ':' && target[2] == '/' {
		return PublishedAsset{}, fmt.Errorf("不能发布 Obsidian 仓库外的本地图片: %s", rawTarget)
	}

	candidates := []string{target}
	articleKey = cleanObjectKey(articleKey)
	if articleKey != "" {
		candidate := cleanObjectKey(path.Join(path.Dir(articleKey), target))
		if candidate != target {
			candidates = append(candidates, candidate)
		}
	}

	for _, candidate := range candidates {
		for _, object := range index.objects {
			if object.Key == candidate {
				return PublishedAsset{SourceKey: object.Key, ETag: object.ETag, PublicURL: publicURL(object)}, nil
			}
		}
	}

	caseInsensitive := []Object{}
	for _, candidate := range candidates {
		for _, object := range index.objects {
			if strings.EqualFold(object.Key, candidate) {
				caseInsensitive = appendUnique(caseInsensitive, object)
			}
		}
	}
	if len(caseInsensitive) == 1 {
		object := caseInsensitive[0]
		return PublishedAsset{SourceKey: object.Key, ETag: object.ETag, PublicURL: publicURL(object)}, nil
	}

	basename := path.Base(target)
	byName := []Object{}
	for _, object := range index.objects {
		if strings.EqualFold(path.Base(object.Key), basename) {
			byName = append(byName, object)
		}
	}
	if len(byName) == 1 {
		object := byName[0]
		return PublishedAsset{SourceKey: object.Key, ETag: object.ETag, PublicURL: publicURL(object)}, nil
	}
	if len(byName) > 1 {
		paths := make([]string, 0, len(byName))
		for _, object := range byName {
			paths = append(paths, object.Key)
		}
		return PublishedAsset{}, fmt.Errorf("图片名称不唯一 %q，请在 Obsidian 中写完整相对路径（候选: %s）", rawTarget, strings.Join(paths, ", "))
	}
	return PublishedAsset{}, fmt.Errorf("S3 中找不到文章引用的图片: %s", rawTarget)
}

func appendUnique(objects []Object, candidate Object) []Object {
	for _, object := range objects {
		if object.Key == candidate.Key {
			return objects
		}
	}
	return append(objects, candidate)
}

// RewriteDocument rewrites Obsidian wiki image embeds and local Markdown image
// destinations. Fenced code blocks and already-absolute/remote URLs are left
// untouched. The returned asset list is de-duplicated by public URL.
func (index *Index) RewriteDocument(content, articleKey string) (string, []PublishedAsset, error) {
	lines := strings.Split(content, "\n")
	inFence := false
	fenceMarker := ""
	assets := map[string]PublishedAsset{}
	errorsFound := []string{}

	for lineIndex, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if inFence {
			continue
		}

		line = rewriteOutsideInlineCode(line, func(segment string) string {
			segment = wikiImagePattern.ReplaceAllStringFunc(segment, func(match string) string {
				parts := wikiImagePattern.FindStringSubmatch(match)
				if len(parts) != 2 {
					return match
				}
				inside := strings.TrimSpace(parts[1])
				pieces := strings.SplitN(inside, "|", 2)
				target := strings.TrimSpace(pieces[0])
				if !IsImageKey(cleanTarget(target)) {
					// Obsidian also uses ![[...]] for note and document embeds.
					// This package deliberately handles images only.
					return match
				}
				alt := strings.TrimSuffix(path.Base(cleanTarget(target)), path.Ext(cleanTarget(target)))
				caption := ""
				if len(pieces) == 2 && strings.TrimSpace(pieces[1]) != "" && !allDigits(strings.TrimSpace(pieces[1])) {
					alt = strings.TrimSpace(pieces[1])
					caption = alt
				}
				asset, err := index.Resolve(target, articleKey)
				if err != nil {
					errorsFound = append(errorsFound, err.Error())
					return match
				}
				assets[asset.PublicURL] = asset
				return markdownImage(alt, asset.PublicURL, caption)
			})

			return markdownImagePattern.ReplaceAllStringFunc(segment, func(match string) string {
				parts := markdownImagePattern.FindStringSubmatch(match)
				if len(parts) != 3 {
					return match
				}
				rawDestination, title := markdownDestination(parts[2])
				if isRemoteOrAbsolute(rawDestination) {
					return match
				}
				asset, err := index.Resolve(rawDestination, articleKey)
				if err != nil {
					errorsFound = append(errorsFound, err.Error())
					return match
				}
				assets[asset.PublicURL] = asset
				return markdownImage(parts[1], asset.PublicURL, title)
			})
		})
		lines[lineIndex] = line
	}

	resultAssets := make([]PublishedAsset, 0, len(assets))
	for _, asset := range assets {
		resultAssets = append(resultAssets, asset)
	}
	sort.Slice(resultAssets, func(i, j int) bool { return resultAssets[i].PublicURL < resultAssets[j].PublicURL })
	if len(errorsFound) > 0 {
		return strings.Join(lines, "\n"), resultAssets, fmt.Errorf("图片引用无效: %s", strings.Join(uniqueStrings(errorsFound), "; "))
	}
	return strings.Join(lines, "\n"), resultAssets, nil
}

func rewriteOutsideInlineCode(line string, rewrite func(string) string) string {
	var result strings.Builder
	start := 0
	inCode := false
	delimiter := ""
	for index := 0; index < len(line); {
		if line[index] != '`' {
			index++
			continue
		}
		end := index + 1
		for end < len(line) && line[end] == '`' {
			end++
		}
		run := line[index:end]
		if !inCode {
			result.WriteString(rewrite(line[start:index]))
			result.WriteString(run)
			inCode = true
			delimiter = run
			start = end
		} else if run == delimiter {
			result.WriteString(line[start:index])
			result.WriteString(run)
			inCode = false
			delimiter = ""
			start = end
		}
		index = end
	}
	if inCode {
		result.WriteString(line[start:])
	} else {
		result.WriteString(rewrite(line[start:]))
	}
	return result.String()
}

func markdownDestination(value string) (string, string) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") {
		if end := strings.Index(value, ">"); end > 0 {
			return strings.TrimSpace(value[1:end]), markdownTitle(value[end+1:])
		}
	}
	// A Markdown title follows the destination after whitespace and a quote.
	// Keep spaces in ordinary Obsidian filenames such as "Pasted image.png".
	for _, marker := range []string{` "`, ` '`, " ("} {
		if index := strings.Index(value, marker); index > 0 {
			return strings.TrimSpace(value[:index]), markdownTitle(value[index+1:])
		}
	}
	return value, ""
}

func markdownTitle(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') || (first == '(' && last == ')') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

func markdownImage(alt, destination, title string) string {
	result := "![" + escapeAlt(alt) + "](" + destination
	if strings.TrimSpace(title) != "" {
		result += ` "` + escapeTitle(strings.TrimSpace(title)) + `"`
	}
	return result + ")"
}

func escapeTitle(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func escapeAlt(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, "]", `\]`)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
