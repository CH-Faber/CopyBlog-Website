package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gosync/ai"
	"gosync/builder"
	"gosync/config"
	"gosync/contentmodel"
	"gosync/s3sync"
	"gosync/taxonomy"
)

var ErrConflict = errors.New("revision conflict")

type Manager struct {
	cfg       *config.Config
	syncer    *s3sync.S3Syncer
	generator *ai.Generator
	mu        sync.RWMutex
	runMu     sync.Mutex
	publishMu sync.Mutex
	jobs      map[string]*Job
}

func NewManager(cfg *config.Config, syncer *s3sync.S3Syncer, generator *ai.Generator) (*Manager, error) {
	manager := &Manager{cfg: cfg, syncer: syncer, generator: generator, jobs: map[string]*Job{}}
	if err := os.MkdirAll(manager.jobsDir(), 0755); err != nil {
		return nil, err
	}
	if err := manager.loadExisting(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) jobsDir() string {
	return filepath.Join(m.cfg.ProjectRootDir, ".gosync", "jobs")
}

func randomID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

func articleID(path string) string {
	// The path is retained in the manifest; a compact deterministic ID is enough here.
	return contentmodel.HashBytes([]byte(strings.ToLower(filepath.ToSlash(path))))[:16]
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (m *Manager) Create(request CreateRequest) (*Job, error) {
	job := &Job{ID: randomID(), Status: StatusQueued, Progress: 0, Message: "任务已创建", ClientID: request.ClientID, CreatedAt: now(), UpdatedAt: now(), Articles: []*ArticleDraft{}, Errors: []string{}}
	for _, file := range request.LocalManifest {
		// Store client hashes later by matching filename; no local content leaves the client.
		_ = file
	}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()
	if err := m.save(job); err != nil {
		return nil, err
	}
	go m.run(job.ID, request.LocalManifest, request.ClientID)
	return cloneJob(job), nil
}

func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (m *Manager) GetArticle(jobID, articleID string) (*ArticleDraft, bool) {
	job, ok := m.Get(jobID)
	if !ok {
		return nil, false
	}
	for _, article := range job.Articles {
		if article.ID == articleID {
			return article, true
		}
	}
	return nil, false
}

func (m *Manager) UpdateArticle(jobID, id string, request UpdateArticleRequest) (*ArticleDraft, error) {
	values, err := taxonomy.Load(m.cfg)
	if err != nil {
		return nil, err
	}
	valid, unknown := values.ValidateTags(request.Metadata.Tags)
	if len(unknown) > 0 {
		return nil, fmt.Errorf("标签尚未批准: %s", strings.Join(unknown, ", "))
	}
	request.Metadata.Tags = valid
	if strings.TrimSpace(request.Metadata.Category) != "" {
		category, ok := values.ValidateCategory(request.Metadata.Category)
		if !ok {
			return nil, fmt.Errorf("分类尚未批准或已停用: %s", request.Metadata.Category)
		}
		request.Metadata.Category = category
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, os.ErrNotExist
	}
	for _, article := range job.Articles {
		if article.ID != id {
			continue
		}
		if article.Revision != request.Revision {
			return nil, ErrConflict
		}
		article.Metadata = request.Metadata
		article.Content = request.Content
		if request.Status != "" {
			article.Status = request.Status
		}
		article.Revision++
		hash, hashErr := contentmodel.HashDocument(contentmodel.Document{Metadata: article.Metadata, Content: article.Content})
		if hashErr != nil {
			return nil, hashErr
		}
		article.CurrentHash = hash
		job.UpdatedAt = now()
		if err := m.saveLocked(job); err != nil {
			return nil, err
		}
		return cloneArticle(article), nil
	}
	return nil, os.ErrNotExist
}

func (m *Manager) run(jobID string, localManifest []LocalFile, clientID string) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	m.setStatus(jobID, StatusSyncing, 5, "正在从 S3 获取文章")

	sourceDir := filepath.Join(m.jobsDir(), jobID, "source")
	if err := m.syncer.SyncArticlesTo(sourceDir, m.cfg.LocalPostsDir); err != nil {
		m.fail(jobID, err)
		return
	}
	m.setStatus(jobID, StatusAnalyzing, 30, "正在解析文章并生成 AI 建议")

	values, err := taxonomy.Load(m.cfg)
	if err != nil {
		m.fail(jobID, err)
		return
	}
	clientHashes := map[string]string{}
	for _, file := range localManifest {
		clientHashes[strings.ToLower(filepath.Base(file.Path))] = file.Hash
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		m.fail(jobID, err)
		return
	}
	articles := []*ArticleDraft{}
	remoteNames := map[string]bool{}
	eligible := []os.DirEntry{}
	for _, entry := range entries {
		if !entry.IsDir() && (strings.EqualFold(filepath.Ext(entry.Name()), ".md") || strings.EqualFold(filepath.Ext(entry.Name()), ".mdx")) {
			eligible = append(eligible, entry)
		}
	}
	for index, entry := range eligible {
		filename := entry.Name()
		remoteNames[strings.ToLower(filename)] = true
		path := filepath.Join(sourceDir, filename)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		doc, parseErr := contentmodel.Parse(string(data))
		if parseErr != nil {
			articles = append(articles, &ArticleDraft{ID: articleID(filename), Path: filename, Filename: filename, Status: ArticleConflict, Revision: 1, Error: parseErr.Error()})
			continue
		}
		info, _ := entry.Info()
		modified := time.Now()
		if info != nil {
			modified = info.ModTime()
		}
		contentmodel.EnsureDefaults(&doc, filename, modified)
		sourceHash := contentmodel.HashBytes(data)

		originalDoc := contentmodel.Document{Metadata: contentmodel.ArticleMetadata{Tags: []string{}}, Content: ""}
		originalHash := ""
		status := ArticleNew
		if published, readPublishedErr := os.ReadFile(filepath.Join(m.cfg.LocalPostsDir, filename)); readPublishedErr == nil {
			originalHash = contentmodel.HashBytes(published)
			status = ArticleModified
			if parsed, parsedErr := contentmodel.Parse(string(published)); parsedErr == nil {
				originalDoc = parsed
				if equivalent, compareErr := contentmodel.EquivalentDocuments(doc, originalDoc); compareErr == nil && equivalent {
					status = ArticleUnchanged
				}
			}
		}

		var suggestion *ai.Suggestion
		articleError := ""
		// Current Obsidian clients run AI locally so their provider key never reaches this service.
		// Legacy clients retain the server-side generator until they are migrated.
		needsAI := clientID != "obsidian" && (status == ArticleNew || strings.TrimSpace(doc.Metadata.Description) == "" || strings.TrimSpace(doc.Metadata.Category) == "" || len(doc.Metadata.Tags) == 0)
		if needsAI {
			suggestion, err = m.generator.SuggestMetadata(filename, doc.Content, values)
			if err != nil {
				articleError = "AI 处理失败: " + err.Error()
				suggestion = &ai.Suggestion{SelectedTags: []string{}, ProposedTags: []ai.ProposedTag{}}
			}
			if doc.Metadata.Description == "" {
				doc.Metadata.Description = suggestion.Description
			}
			if doc.Metadata.Category == "" {
				doc.Metadata.Category = suggestion.Category
			}
			if len(doc.Metadata.Tags) == 0 {
				doc.Metadata.Tags = suggestion.SelectedTags
			}
		}
		currentHash, _ := contentmodel.HashDocument(doc)
		article := &ArticleDraft{ID: articleID(filename), Path: filename, Filename: filename, Status: status, Metadata: doc.Metadata, Content: doc.Content, OriginalMetadata: originalDoc.Metadata, OriginalContent: originalDoc.Content, AISuggestion: suggestion, SourceHash: sourceHash, OriginalHash: originalHash, ClientHash: clientHashes[strings.ToLower(filename)], CurrentHash: currentHash, Revision: 1, Error: articleError}
		articles = append(articles, article)
		progress := 30
		if len(eligible) > 0 {
			progress += int(float64(index+1) / float64(len(eligible)) * 55)
		}
		m.setStatus(jobID, StatusAnalyzing, progress, "正在处理 "+filename)
	}

	// Missing remote files become deletion proposals; they are never deleted here.
	publishedEntries, _ := os.ReadDir(m.cfg.LocalPostsDir)
	for _, entry := range publishedEntries {
		if entry.IsDir() || (!strings.EqualFold(filepath.Ext(entry.Name()), ".md") && !strings.EqualFold(filepath.Ext(entry.Name()), ".mdx")) || remoteNames[strings.ToLower(entry.Name())] {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(m.cfg.LocalPostsDir, entry.Name()))
		if readErr != nil {
			continue
		}
		doc, parseErr := contentmodel.Parse(string(data))
		if parseErr != nil {
			continue
		}
		hash := contentmodel.HashBytes(data)
		articles = append(articles, &ArticleDraft{ID: articleID(entry.Name()), Path: entry.Name(), Filename: entry.Name(), Status: ArticleDeleted, Metadata: doc.Metadata, Content: doc.Content, OriginalMetadata: doc.Metadata, OriginalContent: doc.Content, SourceHash: "", OriginalHash: hash, ClientHash: clientHashes[strings.ToLower(entry.Name())], CurrentHash: hash, Revision: 1})
	}
	sort.Slice(articles, func(i, j int) bool { return articles[i].Filename < articles[j].Filename })

	m.mu.Lock()
	if job, ok := m.jobs[jobID]; ok {
		job.Articles = articles
		job.Status = StatusAwaitingReview
		job.Progress = 100
		job.Message = "处理完成，等待审核"
		job.UpdatedAt = now()
		job.SourceCommit = gitHead(m.cfg.ProjectRootDir)
		_ = m.saveLocked(job)
	}
	m.mu.Unlock()
}

func (m *Manager) Publish(jobID string, request PublishRequest) (*PublishResponse, error) {
	m.publishMu.Lock()
	defer m.publishMu.Unlock()

	values, err := taxonomy.Load(m.cfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return nil, os.ErrNotExist
	}
	selected := map[string]PublishArticleRequest{}
	for _, item := range request.Articles {
		selected[item.ID] = item
	}
	files := map[string]*[]byte{}
	expected := map[string]string{}
	publishedTitles := []string{}
	for _, article := range job.Articles {
		item, include := selected[article.ID]
		if !include {
			continue
		}
		if article.Revision != item.Revision || article.CurrentHash != item.Hash {
			m.mu.Unlock()
			return nil, ErrConflict
		}
		if article.Status == ArticleDeleted {
			files[article.Filename] = nil
			expected[article.Filename] = article.OriginalHash
			publishedTitles = append(publishedTitles, "删除 "+article.Metadata.Title)
			continue
		}
		valid, unknown := values.ValidateTags(article.Metadata.Tags)
		if len(unknown) > 0 {
			m.mu.Unlock()
			return nil, fmt.Errorf("文章 %s 含未批准标签: %s", article.Metadata.Title, strings.Join(unknown, ", "))
		}
		article.Metadata.Tags = valid
		category, categoryOK := values.ValidateCategory(article.Metadata.Category)
		if !categoryOK {
			m.mu.Unlock()
			if strings.TrimSpace(article.Metadata.Category) == "" {
				return nil, fmt.Errorf("文章 %s 尚未选择分类", article.Metadata.Title)
			}
			return nil, fmt.Errorf("文章 %s 的分类尚未批准或已停用: %s", article.Metadata.Title, article.Metadata.Category)
		}
		article.Metadata.Category = category
		data, serializeErr := contentmodel.Serialize(contentmodel.Document{Metadata: article.Metadata, Content: article.Content})
		if serializeErr != nil {
			m.mu.Unlock()
			return nil, serializeErr
		}
		files[article.Filename] = &data
		expected[article.Filename] = article.OriginalHash
		publishedTitles = append(publishedTitles, article.Metadata.Title)
	}
	if len(files) == 0 {
		m.mu.Unlock()
		return nil, fmt.Errorf("没有选择可发布的文章")
	}
	if request.IncludeTaxonomy {
		if err := validateFinalTaxonomyState(m.cfg.LocalPostsDir, values, job.Articles, selected); err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}
	job.Status = StatusPublishing
	job.Message = "正在提交到 deploy"
	job.UpdatedAt = now()
	_ = m.saveLocked(job)
	m.mu.Unlock()

	if request.IncludeTaxonomy {
		data, taxonomyErr := taxonomy.PublishedJSON(values)
		if taxonomyErr != nil {
			m.fail(jobID, taxonomyErr)
			return nil, taxonomyErr
		}
		files["../../data/content-taxonomy.json"] = &data
	}
	message := fmt.Sprintf("Publish %d approved posts from Obsidian", len(request.Articles))
	sha, publishErr := builder.PublishFiles(m.cfg, files, expected, message)
	if publishErr != nil {
		m.fail(jobID, publishErr)
		return nil, publishErr
	}

	m.mu.Lock()
	job = m.jobs[jobID]
	job.Status = StatusPublished
	job.Message = "已推送 deploy"
	job.PublishedSHA = sha
	job.UpdatedAt = now()
	for _, article := range job.Articles {
		if _, include := selected[article.ID]; include {
			article.Status = ArticlePublished
		}
	}
	_ = m.saveLocked(job)
	m.mu.Unlock()
	_ = publishedTitles
	return &PublishResponse{CommitSHA: sha}, nil
}

// validateFinalTaxonomyState overlays the selected review changes on the currently
// published posts, then validates the complete result before any Git write occurs.
func validateFinalTaxonomyState(postsDir string, values *taxonomy.Taxonomy, articles []*ArticleDraft, selected map[string]PublishArticleRequest) error {
	final := map[string]contentmodel.ArticleMetadata{}
	entries, err := os.ReadDir(postsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || (!strings.EqualFold(filepath.Ext(entry.Name()), ".md") && !strings.EqualFold(filepath.Ext(entry.Name()), ".mdx")) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(postsDir, entry.Name()))
		if readErr != nil {
			return readErr
		}
		doc, parseErr := contentmodel.Parse(string(data))
		if parseErr != nil {
			return fmt.Errorf("解析已发布文章 %s 失败: %w", entry.Name(), parseErr)
		}
		final[entry.Name()] = doc.Metadata
	}
	for _, article := range articles {
		if _, include := selected[article.ID]; !include {
			continue
		}
		if article.Status == ArticleDeleted {
			delete(final, article.Filename)
		} else {
			final[article.Filename] = article.Metadata
		}
	}
	for filename, metadata := range final {
		title := strings.TrimSpace(metadata.Title)
		if title == "" {
			title = filename
		}
		category, ok := values.ValidateCategory(metadata.Category)
		if !ok {
			if strings.TrimSpace(metadata.Category) == "" {
				return fmt.Errorf("无法发布：文章《%s》尚未选择分类", title)
			}
			return fmt.Errorf("无法发布：文章《%s》仍在使用未批准或已停用分类“%s”", title, metadata.Category)
		}
		_ = category
		_, unknown := values.ValidateTags(metadata.Tags)
		if len(unknown) > 0 {
			return fmt.Errorf("无法发布：文章《%s》仍在使用未批准或已停用标签“%s”，请将该文章加入本次发布", title, strings.Join(unknown, "、"))
		}
	}
	return nil
}

func (m *Manager) setStatus(id string, status Status, progress int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[id]; ok {
		job.Status, job.Progress, job.Message, job.UpdatedAt = status, progress, message, now()
		_ = m.saveLocked(job)
	}
}

func (m *Manager) fail(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[id]; ok {
		job.Status, job.Message, job.UpdatedAt = StatusFailed, err.Error(), now()
		job.Errors = append(job.Errors, err.Error())
		_ = m.saveLocked(job)
	}
}

func (m *Manager) save(job *Job) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.saveLocked(job)
}

func (m *Manager) saveLocked(job *Job) error {
	dir := filepath.Join(m.jobsDir(), job.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	temp := filepath.Join(dir, "job.json.tmp")
	final := filepath.Join(dir, "job.json")
	if err := os.WriteFile(temp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(temp, final)
}

func (m *Manager) loadExisting() error {
	entries, err := os.ReadDir(m.jobsDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(m.jobsDir(), entry.Name(), "job.json"))
		if readErr != nil {
			continue
		}
		var job Job
		if json.Unmarshal(data, &job) == nil {
			if job.Status == StatusSyncing || job.Status == StatusAnalyzing || job.Status == StatusPublishing {
				job.Status = StatusFailed
				job.Message = "服务重启中断了任务，请重新执行"
			}
			m.jobs[job.ID] = &job
		}
	}
	return nil
}

func cloneJob(job *Job) *Job {
	data, _ := json.Marshal(job)
	var result Job
	_ = json.Unmarshal(data, &result)
	return &result
}

func cloneArticle(article *ArticleDraft) *ArticleDraft {
	data, _ := json.Marshal(article)
	var result ArticleDraft
	_ = json.Unmarshal(data, &result)
	return &result
}

func gitHead(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
