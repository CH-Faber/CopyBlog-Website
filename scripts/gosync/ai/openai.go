package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gosync/config"
	"gosync/taxonomy"

	openai "github.com/sashabaranov/go-openai"
)

var (
	reYAMLTitle     = regexp.MustCompile(`(?m)^title:\s*\S`)
	reYAMLPublished = regexp.MustCompile(`(?m)^published:\s*\S`)
)

// splitFrontmatter 解析首块 YAML（以首行 --- 与下一个换行后的 --- 为界）。
func splitFrontmatter(s string) (fmBlock, body string, ok bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", s, false
	}
	rest := s[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", s, false
	}
	fmBlock = strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+4:])
	return fmBlock, body, true
}

func hasCompleteAstroFrontmatter(fmBlock string) bool {
	return reYAMLTitle.MatchString(fmBlock) && reYAMLPublished.MatchString(fmBlock)
}

type Generator struct {
	client *openai.Client
	cfg    *config.Config
}

func NewGenerator(cfg *config.Config) *Generator {
	clientConfig := openai.DefaultConfig(cfg.AIApiKey)
	clientConfig.BaseURL = cfg.AIBaseURL
	return &Generator{
		client: openai.NewClientWithConfig(clientConfig),
		cfg:    cfg,
	}
}

func (g *Generator) ProcessMissingFrontmatters() error {
	files, err := os.ReadDir(g.cfg.LocalPostsDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		path := filepath.Join(g.cfg.LocalPostsDir, file.Name())
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(contentBytes)
		trimmed := strings.TrimLeft(content, " \t\r\n")

		fmBlock, body, hasFM := splitFrontmatter(trimmed)
		if hasFM && hasCompleteAstroFrontmatter(fmBlock) {
			continue
		}

		bodyForAI := trimmed
		if hasFM {
			bodyForAI = body
			if strings.TrimSpace(bodyForAI) == "" {
				bodyForAI = trimmed
			}
		}

		if g.cfg.AIApiKey == "" {
			log.Printf("[%s] 补全默认 frontmatter（未配置 AI_API_KEY）\n", file.Name())
			fmString := buildFmString(&FMResponse{}, file.Name(), path)
			finalContent := fmString + "\n\n" + strings.TrimSpace(bodyForAI)
			if err := os.WriteFile(path, []byte(finalContent), 0644); err != nil {
				log.Printf("write %s: %v\n", file.Name(), err)
			}
			continue
		}

		log.Printf("[%s] 🤖 Processing with AI...\n", file.Name())
		fmData, err := g.generateFrontmatter(file.Name(), bodyForAI)
		if err != nil {
			log.Printf("AI generation failed for %s: %v\n", file.Name(), err)
			continue
		}

		fmString := buildFmString(fmData, file.Name(), path)
		finalContent := fmString + "\n\n" + strings.TrimSpace(bodyForAI)
		if err := os.WriteFile(path, []byte(finalContent), 0644); err != nil {
			log.Printf("write %s: %v\n", file.Name(), err)
		}
	}

	return nil
}

type FMResponse struct {
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}

type ProposedTag struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ProposedCategory struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type Suggestion struct {
	Description      string            `json:"description"`
	Category         string            `json:"category"`
	ProposedCategory *ProposedCategory `json:"proposedCategory,omitempty"`
	SelectedTags     []string          `json:"selectedTags"`
	ProposedTags     []ProposedTag     `json:"proposedTags"`
}

// SuggestMetadata generates reviewable metadata. Existing tags are strictly validated
// against the taxonomy; unknown model output is moved into proposedTags for approval.
func (g *Generator) SuggestMetadata(filename, content string, values *taxonomy.Taxonomy) (*Suggestion, error) {
	if g.cfg.AIApiKey == "" {
		return &Suggestion{SelectedTags: []string{}, ProposedTags: []ProposedTag{}}, nil
	}
	snippet := content
	if len(snippet) > 6000 {
		snippet = snippet[:6000]
	}

	categoryLines := []string{}
	for _, category := range values.Categories {
		if category.Enabled {
			categoryLines = append(categoryLines, fmt.Sprintf("- %s：%s", category.Name, category.Description))
		}
	}
	tagLines := []string{}
	for _, tag := range values.AllowedTags() {
		tagLines = append(tagLines, fmt.Sprintf("- %s：%s", tag.Name, tag.Description))
	}
	prompt := fmt.Sprintf(`你是博客文章元数据审核助手。只输出一个 JSON 对象，不要 Markdown 围栏或说明。
输出格式：
{"description":"1-2句中文摘要","category":"只能填写已有分类，没有合适分类时留空","proposedCategory":{"name":"建议的新分类","reason":"为什么现有分类不合适"},"selectedTags":["只能来自已有标签库"],"proposedTags":[{"name":"建议的新标签","reason":"为什么需要"}]}

规则：category 只能从已有分类中选择；没有合适分类时 category 必须为空，并在 proposedCategory 中提出一个新分类，否则 proposedCategory 为 null。selectedTags 只能从已有标签库中选择 3-6 个；确实缺少合适标签时放入 proposedTags，禁止把新标签放进 selectedTags。不要输出标题和发布时间。

已有分类：
%s

已有标签库：
%s

文件名：%s
正文：
%s`, strings.Join(categoryLines, "\n"), strings.Join(tagLines, "\n"), filename, snippet)

	resp, err := CreateChatCompletionCompat(context.TODO(), g.client, openai.ChatCompletionRequest{
		Model: g.cfg.AIModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "你只返回合法 JSON。"},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	}, 2048)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	reply := strings.TrimSpace(resp.Choices[0].Message.Content)
	reply = strings.ReplaceAll(reply, "```json", "")
	reply = strings.ReplaceAll(reply, "```", "")
	if start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}"); start >= 0 && end >= start {
		reply = reply[start : end+1]
	}
	var result Suggestion
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	categoryName := strings.TrimSpace(result.Category)
	if canonical, ok := values.ValidateCategory(categoryName); ok {
		result.Category = canonical
	} else {
		result.Category = ""
		if categoryName != "" && (result.ProposedCategory == nil || strings.TrimSpace(result.ProposedCategory.Name) == "") {
			result.ProposedCategory = &ProposedCategory{Name: categoryName, Reason: "AI 返回了分类库外的分类"}
		}
	}
	if result.ProposedCategory != nil {
		result.ProposedCategory.Name = strings.TrimSpace(result.ProposedCategory.Name)
		if canonical, ok := values.ValidateCategory(result.ProposedCategory.Name); ok {
			if result.Category == "" {
				result.Category = canonical
			}
			result.ProposedCategory = nil
		} else if result.ProposedCategory.Name == "" {
			result.ProposedCategory = nil
		}
	}
	valid, unknown := values.ValidateTags(result.SelectedTags)
	if len(valid) > 6 {
		valid = valid[:6]
	}
	result.SelectedTags = valid
	knownProposals := map[string]bool{}
	for _, proposal := range result.ProposedTags {
		knownProposals[strings.ToLower(strings.TrimSpace(proposal.Name))] = true
	}
	for _, name := range unknown {
		if !knownProposals[strings.ToLower(name)] {
			result.ProposedTags = append(result.ProposedTags, ProposedTag{Name: name, Reason: "AI 返回了标签库外的标签"})
		}
	}
	if result.SelectedTags == nil {
		result.SelectedTags = []string{}
	}
	if result.ProposedTags == nil {
		result.ProposedTags = []ProposedTag{}
	}
	return &result, nil
}

const (
	frontmatterTokensFirst  = 2048
	frontmatterTokensRetry  = 4096
	frontmatterRetryUserAdd = `

【再次请求】上一段输出无法作为合法 JSON 解析（或内容为空）。请只输出一个 JSON 对象，从 { 开始到 } 结束；键只能是 description、category、tags；不要使用 markdown 代码块或其它文字。`
)

func (g *Generator) generateFrontmatter(filename, content string) (*FMResponse, error) {
	snippet := content
	if len(snippet) > 6000 {
		snippet = snippet[:6000]
	}

	userBase := fmt.Sprintf(userPromptFrontmatterFmt, filename, snippet)
	limits := []int{frontmatterTokensFirst, frontmatterTokensRetry}
	var lastErr error

	for attempt, lim := range limits {
		userPrompt := userBase
		if attempt > 0 {
			userPrompt = userBase + frontmatterRetryUserAdd
		}
		data, err := g.chatFrontmatterOnce(userPrompt, lim)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !isRetriableFrontmatterErr(err) {
			break
		}
	}

	return nil, lastErr
}

func isRetriableFrontmatterErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "JSON parse") ||
		strings.Contains(s, "empty model content") ||
		strings.Contains(s, "no choices") ||
		strings.Contains(s, "output truncated")
}

func (g *Generator) chatFrontmatterOnce(userPrompt string, outputLimit int) (*FMResponse, error) {
	resp, err := CreateChatCompletionCompat(
		context.TODO(),
		g.client,
		openai.ChatCompletionRequest{
			Model: g.cfg.AIModel,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: systemPromptFrontmatter},
				{Role: openai.ChatMessageRoleUser, Content: userPrompt},
			},
		},
		outputLimit,
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	ch := resp.Choices[0]
	if ch.FinishReason == openai.FinishReasonLength {
		return nil, fmt.Errorf("output truncated (finish_reason=length)")
	}

	reply := strings.TrimSpace(ch.Message.Content)
	reply = strings.ReplaceAll(reply, "```json", "")
	reply = strings.ReplaceAll(reply, "```", "")
	reply = strings.TrimSpace(reply)

	if reply == "" {
		return nil, fmt.Errorf("empty model content (finish_reason=%s)", ch.FinishReason)
	}

	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start != -1 && end != -1 && end >= start {
		reply = reply[start : end+1]
	}

	var data FMResponse
	if err := json.Unmarshal([]byte(reply), &data); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return &data, nil
}

func buildFmString(data *FMResponse, filename, fullPath string) string {
	title := strings.TrimSuffix(filename, ".md")
	title = strings.ReplaceAll(title, "\"", "\\\"")
	desc := strings.ReplaceAll(data.Description, "\"", "\\\"")

	tagsJSON, err := json.Marshal(data.Tags)
	if err != nil || string(tagsJSON) == "null" {
		tagsJSON = []byte("[]")
	}

	published := time.Now()
	if info, err := os.Stat(fullPath); err == nil {
		published = info.ModTime()
	}

	lines := []string{
		"---",
		fmt.Sprintf(`title: "%s"`, title),
		fmt.Sprintf(`published: %s`, published.Format("2006-01-02T15:04:05.000Z")),
		fmt.Sprintf(`description: "%s"`, desc),
		fmt.Sprintf(`category: "%s"`, data.Category),
		fmt.Sprintf(`tags: %s`, string(tagsJSON)),
		"---",
	}

	return strings.Join(lines, "\n")
}
