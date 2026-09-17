package s3sync

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gosync/config"
	"gosync/media"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Syncer struct {
	client        *s3.Client
	cfg           *config.Config
	downloadCount int
}

const maxPublishedImageBytes int64 = 50 << 20

// ListImageObjects returns metadata only. Images are downloaded later, and
// only when an explicitly approved article actually references them.
func (s *S3Syncer) ListImageObjects() ([]media.Object, error) {
	ctx := context.TODO()
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.S3BucketName),
	})
	objects := []media.Object{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list image objects: %w", err)
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if media.IsImageKey(key) {
				objects = append(objects, media.Object{Key: key, ETag: strings.Trim(aws.ToString(object.ETag), `"`)})
			}
		}
	}
	return objects, nil
}

// DownloadImage verifies that the reviewed S3 object has not changed and caps
// memory use before returning bytes for the website's public asset directory.
func (s *S3Syncer) DownloadImage(key, expectedETag string) ([]byte, error) {
	if !media.IsImageKey(key) {
		return nil, fmt.Errorf("refusing to download unsupported image object: %s", key)
	}
	out, err := s.client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.S3BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("download image %s: %w", key, err)
	}
	defer out.Body.Close()
	actualETag := strings.Trim(aws.ToString(out.ETag), `"`)
	expectedETag = strings.Trim(strings.TrimSpace(expectedETag), `"`)
	if expectedETag != "" && actualETag != "" && actualETag != expectedETag {
		return nil, fmt.Errorf("image changed after review started, please create a new sync task: %s", key)
	}
	if out.ContentLength != nil && *out.ContentLength > maxPublishedImageBytes {
		return nil, fmt.Errorf("image exceeds the 50 MiB publish limit: %s", key)
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, maxPublishedImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image %s: %w", key, err)
	}
	if int64(len(data)) > maxPublishedImageBytes {
		return nil, fmt.Errorf("image exceeds the 50 MiB publish limit: %s", key)
	}
	return data, nil
}

func NewS3Syncer(cfg *config.Config) (*S3Syncer, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           cfg.S3Endpoint,
			SigningRegion: cfg.S3Region,
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &S3Syncer{
		client: client,
		cfg:    cfg,
	}, nil
}

func (s *S3Syncer) SyncArticles() error {
	s.downloadCount = 0
	ctx := context.TODO()

	os.MkdirAll(s.cfg.LocalPostsDir, os.ModePerm)

	log.Printf("Starting sync from %s/%s to %s\n", s.cfg.S3BucketName, s.cfg.S3Prefix, s.cfg.LocalPostsDir)

	// 桶内当前存在的 .md 文件名（basename），用于删除已在 Obsidian/S3 侧移除的本地副本
	remoteMD := make(map[string]struct{})

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.S3BucketName),
		Prefix: aws.String(s.cfg.S3Prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(key, ".md") {
				continue
			}

			filename := filepath.Base(key)
			remoteMD[filename] = struct{}{}
			localPath := filepath.Join(s.cfg.LocalPostsDir, filename)
			s3MTime := obj.LastModified

			if fileInfo, err := os.Stat(localPath); err == nil {
				localMTime := fileInfo.ModTime()
				if !s3MTime.After(localMTime) {
					continue // Local is newer or equal
				}

				log.Printf("[%s] ♻️  Updating...\n", filename)
				s.downloadAndMerge(ctx, key, localPath)
			} else {
				log.Printf("[%s] 🆕 Downloading new article...\n", filename)
				s.downloadFile(ctx, key, localPath)
			}
		}
	}

	removed := 0
	entries, err := os.ReadDir(s.cfg.LocalPostsDir)
	if err != nil {
		return fmt.Errorf("read posts dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		name := e.Name()
		if _, ok := remoteMD[name]; ok {
			continue
		}
		p := filepath.Join(s.cfg.LocalPostsDir, name)
		log.Printf("[%s] 🗑️  Removing (no longer under S3 prefix)...\n", name)
		if err := os.Remove(p); err != nil {
			log.Printf("remove %s: %v\n", name, err)
			continue
		}
		removed++
	}

	log.Printf("Sync completed. Downloaded/updated %d, removed locally %d.\n", s.downloadCount, removed)
	return nil
}

// SyncContentTo downloads Markdown into an isolated job directory while preserving
// paths below S3Prefix. Root files and posts/* are articles; thoughts/* and
// flashes/* are flashes. A flat file may also opt into the flash collection with
// frontmatter `type: thought` or `type: flash` (handled later by the job manager).
// Published frontmatter is used only as a fallback and published files are never modified.
func (s *S3Syncer) SyncContentTo(destination, publishedPostsDir, publishedThoughtsDir string) error {
	ctx := context.TODO()
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.S3BucketName),
		Prefix: aws.String(s.cfg.S3Prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(strings.ToLower(key), ".md") && !strings.HasSuffix(strings.ToLower(key), ".mdx") {
				continue
			}
			relativeKey := strings.TrimPrefix(strings.ReplaceAll(key, `\`, "/"), strings.TrimRight(strings.ReplaceAll(s.cfg.S3Prefix, `\`, "/"), "/")+"/")
			relativeKey = strings.TrimLeft(relativeKey, "/")
			cleanRelative := filepath.Clean(filepath.FromSlash(relativeKey))
			if cleanRelative == "." || filepath.IsAbs(cleanRelative) || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("refusing unsafe S3 content path: %s", key)
			}
			destinationPath := filepath.Join(destination, cleanRelative)
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
				return err
			}
			tempPath := destinationPath + ".download"
			if err := s.downloadFileImpl(ctx, key, tempPath); err != nil {
				return fmt.Errorf("download %s: %w", key, err)
			}
			remoteBytes, err := os.ReadFile(tempPath)
			_ = os.Remove(tempPath)
			if err != nil {
				return err
			}
			remoteFM, remoteBody := extractFrontmatter(string(remoteBytes))
			finalContent := string(remoteBytes)
			if remoteFM == "" {
				parts := strings.Split(filepath.ToSlash(cleanRelative), "/")
				publishedDir := publishedPostsDir
				if len(parts) > 1 && (strings.EqualFold(parts[0], "thoughts") || strings.EqualFold(parts[0], "flashes") || parts[0] == "闪念") {
					publishedDir = publishedThoughtsDir
				}
				publishedBytes, readErr := os.ReadFile(filepath.Join(publishedDir, filepath.Base(cleanRelative)))
				if readErr == nil {
					publishedFM, _ := extractFrontmatter(string(publishedBytes))
					if publishedFM != "" {
						finalContent = publishedFM + "\n\n" + remoteBody
					}
				}
			}
			if err := os.WriteFile(destinationPath, []byte(finalContent), 0644); err != nil {
				return err
			}
			if obj.LastModified != nil {
				_ = os.Chtimes(destinationPath, time.Now(), *obj.LastModified)
			}
		}
	}
	return nil
}

// SyncArticlesTo is kept for older callers. It retains the historical article-only behavior.
func (s *S3Syncer) SyncArticlesTo(destination, publishedPostsDir string) error {
	return s.SyncContentTo(destination, publishedPostsDir, filepath.Join(filepath.Dir(publishedPostsDir), "thoughts"))
}

func extractFrontmatter(content string) (fm string, body string) {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if strings.HasPrefix(trimmed, "---") {
		parts := strings.SplitN(trimmed, "---", 3)
		if len(parts) >= 3 {
			fm = "---" + parts[1] + "---"
			body = strings.TrimLeft(parts[2], " \t\r\n")
			return
		}
	}
	return "", content
}

func (s *S3Syncer) downloadAndMerge(ctx context.Context, key, localPath string) {
	localContentBytes, _ := os.ReadFile(localPath)
	existingFm, _ := extractFrontmatter(string(localContentBytes))

	tempPath := localPath + ".tmp"
	err := s.downloadFileImpl(ctx, key, tempPath)
	if err != nil {
		log.Printf("Failed to download %s: %v\n", key, err)
		return
	}
	defer os.Remove(tempPath)

	newContentBytes, _ := os.ReadFile(tempPath)
	newFm, newBody := extractFrontmatter(string(newContentBytes))

	var finalContent string
	if newFm != "" {
		finalContent = newFm + "\n\n" + newBody
	} else if existingFm != "" {
		finalContent = existingFm + "\n\n" + newBody
	} else {
		finalContent = newBody
	}

	os.WriteFile(localPath, []byte(finalContent), 0644)
	s.downloadCount++
}

func (s *S3Syncer) downloadFile(ctx context.Context, key, localPath string) {
	err := s.downloadFileImpl(ctx, key, localPath)
	if err != nil {
		log.Printf("Failed to download %s: %v\n", key, err)
	} else {
		s.downloadCount++
	}
}

func (s *S3Syncer) downloadFileImpl(ctx context.Context, key, dest string) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.S3BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()

	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, out.Body)
	// Apply S3 modification time
	if out.LastModified != nil {
		os.Chtimes(dest, time.Now(), *out.LastModified)
	}

	return err
}
