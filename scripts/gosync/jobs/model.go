package jobs

import (
	"gosync/ai"
	"gosync/contentmodel"
)

type Status string

const (
	StatusQueued         Status = "queued"
	StatusSyncing        Status = "syncing"
	StatusAnalyzing      Status = "analyzing"
	StatusAwaitingReview Status = "awaiting_review"
	StatusPublishing     Status = "publishing"
	StatusPublished      Status = "published"
	StatusFailed         Status = "failed"
)

type ArticleStatus string

const (
	ArticleNew       ArticleStatus = "new"
	ArticleModified  ArticleStatus = "modified"
	ArticleUnchanged ArticleStatus = "unchanged"
	ArticleDeleted   ArticleStatus = "deleted"
	ArticleConflict  ArticleStatus = "conflict"
	ArticleApproved  ArticleStatus = "approved"
	ArticleRejected  ArticleStatus = "rejected"
	ArticlePublished ArticleStatus = "published"
)

type LocalFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type CreateRequest struct {
	ClientID      string      `json:"clientId"`
	LocalManifest []LocalFile `json:"localManifest"`
}

type ArticleDraft struct {
	ID               string                       `json:"id"`
	Path             string                       `json:"path"`
	Filename         string                       `json:"filename"`
	Status           ArticleStatus                `json:"status"`
	Metadata         contentmodel.ArticleMetadata `json:"metadata"`
	Content          string                       `json:"content"`
	OriginalMetadata contentmodel.ArticleMetadata `json:"originalMetadata"`
	OriginalContent  string                       `json:"originalContent"`
	AISuggestion     *ai.Suggestion               `json:"aiSuggestion,omitempty"`
	SourceHash       string                       `json:"sourceHash"`
	OriginalHash     string                       `json:"originalHash,omitempty"`
	ClientHash       string                       `json:"clientHash,omitempty"`
	CurrentHash      string                       `json:"currentHash"`
	Revision         int                          `json:"revision"`
	Error            string                       `json:"error,omitempty"`
}

type Job struct {
	ID           string          `json:"id"`
	Status       Status          `json:"status"`
	Progress     int             `json:"progress"`
	Message      string          `json:"message"`
	ClientID     string          `json:"clientId,omitempty"`
	SourceCommit string          `json:"sourceCommit,omitempty"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
	PublishedSHA string          `json:"publishedSha,omitempty"`
	Articles     []*ArticleDraft `json:"articles,omitempty"`
	Errors       []string        `json:"errors,omitempty"`
}

type UpdateArticleRequest struct {
	Revision int                          `json:"revision"`
	Metadata contentmodel.ArticleMetadata `json:"metadata"`
	Content  string                       `json:"content"`
	Status   ArticleStatus                `json:"status,omitempty"`
}

type PublishArticleRequest struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Hash     string `json:"hash"`
}

type PublishRequest struct {
	Articles        []PublishArticleRequest `json:"articles"`
	IncludeTaxonomy bool                    `json:"includeTaxonomy"`
}

type PublishResponse struct {
	CommitSHA string `json:"commitSha"`
}
