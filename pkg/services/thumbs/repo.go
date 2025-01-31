package thumbs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/models"
	dashboardthumbs "github.com/grafana/grafana/pkg/services/dashboard_thumbs"
	"github.com/grafana/grafana/pkg/services/searchV2"
)

func newThumbnailRepo(thumbsService dashboardthumbs.Service, search searchV2.SearchService) thumbnailRepo {
	repo := &sqlThumbnailRepository{
		store:  thumbsService,
		search: search,
		log:    log.New("thumbnails_repo"),
	}
	return repo
}

type sqlThumbnailRepository struct {
	store  dashboardthumbs.Service
	search searchV2.SearchService
	log    log.Logger
}

func (r *sqlThumbnailRepository) saveFromFile(ctx context.Context, filePath string, meta dashboardthumbs.DashboardThumbnailMeta, dashboardVersion int, dsUids []string) (int64, error) {
	// the filePath variable is never set by the user. it refers to a temporary file created either in
	//   1. thumbs/service.go, when user uploads a thumbnail
	//   2. the rendering service, when image-renderer returns a screenshot

	if !filepath.IsAbs(filePath) {
		r.log.Error("Received relative path", "dashboardUID", meta.DashboardUID, "err", filePath)
		return 0, errors.New("relative paths are not supported")
	}

	content, err := os.ReadFile(filepath.Clean(filePath))

	if err != nil {
		r.log.Error("error reading file", "dashboardUID", meta.DashboardUID, "err", err)
		return 0, err
	}

	return r.saveFromBytes(ctx, content, getMimeType(filePath), meta, dashboardVersion, dsUids)
}

func getMimeType(filePath string) string {
	if strings.HasSuffix(filePath, ".webp") {
		return "image/webp"
	}

	return "image/png"
}

func (r *sqlThumbnailRepository) saveFromBytes(ctx context.Context, content []byte, mimeType string, meta dashboardthumbs.DashboardThumbnailMeta, dashboardVersion int, dsUids []string) (int64, error) {
	cmd := &dashboardthumbs.SaveDashboardThumbnailCommand{
		DashboardThumbnailMeta: meta,
		Image:                  content,
		MimeType:               mimeType,
		DashboardVersion:       dashboardVersion,
		DatasourceUIDs:         dsUids,
	}

	_, err := r.store.SaveThumbnail(ctx, cmd)
	if err != nil {
		r.log.Error("Error saving to the db", "dashboardUID", meta.DashboardUID, "err", err)
		return 0, err
	}

	return cmd.Result.Id, nil
}

func (r *sqlThumbnailRepository) updateThumbnailState(ctx context.Context, state dashboardthumbs.ThumbnailState, meta dashboardthumbs.DashboardThumbnailMeta) error {
	return r.store.UpdateThumbnailState(ctx, &dashboardthumbs.UpdateThumbnailStateCommand{
		State:                  state,
		DashboardThumbnailMeta: meta,
	})
}

func (r *sqlThumbnailRepository) getThumbnail(ctx context.Context, meta dashboardthumbs.DashboardThumbnailMeta) (*dashboardthumbs.DashboardThumbnail, error) {
	query := &dashboardthumbs.GetDashboardThumbnailCommand{
		DashboardThumbnailMeta: meta,
	}
	return r.store.GetThumbnail(ctx, query)
}

func (r *sqlThumbnailRepository) findDashboardsWithStaleThumbnails(ctx context.Context, theme models.Theme, kind dashboardthumbs.ThumbnailKind) ([]*dashboardthumbs.DashboardWithStaleThumbnail, error) {
	return r.store.FindDashboardsWithStaleThumbnails(ctx, &dashboardthumbs.FindDashboardsWithStaleThumbnailsCommand{
		IncludeManuallyUploadedThumbnails: false,
		IncludeThumbnailsWithEmptyDsUIDs:  !r.search.IsDisabled(),
		Theme:                             theme,
		Kind:                              kind,
	})
}

func (r *sqlThumbnailRepository) doThumbnailsExist(ctx context.Context) (bool, error) {
	cmd := &dashboardthumbs.FindDashboardThumbnailCountCommand{}
	count, err := r.store.FindThumbnailCount(ctx, cmd)
	if err != nil {
		r.log.Error("Error finding thumbnails", "err", err)
		return false, err
	}
	return count > 0, err
}
