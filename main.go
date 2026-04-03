//go:build wasip1
// +build wasip1

package main

import (
	"context"
	"embed"
	"log/slog"

	"mimusic-plugin-musictag/handlers"
	"mimusic-plugin-musictag/scraper"

	"github.com/knqyf263/go-plugin/types/known/emptypb"
	"github.com/mimusic-org/plugin/api/pbplugin"
	"github.com/mimusic-org/plugin/api/plugin"
)

func main() {}

// Plugin 插件结构体
type Plugin struct {
	Version string

	staticHandler  *plugin.StaticHandler
	scraperManager *scraper.Manager
	scraperHandler *handlers.ScraperHandler
}

func init() {
	plugin.RegisterPlugin(&Plugin{
		Version: "2026.4.3",
	})
}

// GetPluginInfo 返回插件元数据
func (p *Plugin) GetPluginInfo(ctx context.Context, request *emptypb.Empty) (*pbplugin.GetPluginInfoResponse, error) {
	return &pbplugin.GetPluginInfoResponse{
		Success: true,
		Message: "成功获取插件信息",
		Info: &pbplugin.PluginInfo{
			Name:        "音乐标签刮削",
			Version:     p.Version,
			Description: "从其他平台提取歌曲元数据（标题、专辑、艺术家、歌词、封面）",
			Author:      "MiMusic Team",
			Homepage:    "https://github.com/mimusic-org/mimusic",
			EntryPath:   "/musictag",
		},
	}, nil
}

//go:embed static/*
var staticFS embed.FS

// Init 初始化插件
func (p *Plugin) Init(ctx context.Context, request *pbplugin.InitRequest) (*emptypb.Empty, error) {
	slog.Info("正在初始化音乐标签刮削插件", "version", p.Version)

	// 初始化管理器
	p.scraperManager = scraper.NewManager()

	// 初始化处理器
	p.scraperHandler = handlers.NewScraperHandler(p.scraperManager)

	// 获取路由管理器
	rm := plugin.GetRouterManager()

	// 初始化处理器（静态文件处理器需要 rm 来注册路由）
	p.staticHandler = plugin.NewStaticHandler(staticFS, "/musictag", rm, ctx)

	// API 接口（需要认证）
	// 注册路由（使用 EntryPath，不需要 /api/v1/plugin/ 前缀）
	rm.RegisterRouter(ctx, "POST", "/musictag/api/scrape/batch", p.scraperHandler.HandleBatchScrape, true)
	rm.RegisterRouter(ctx, "GET", "/musictag/api/scrape/status", p.scraperHandler.HandleGetStatus, true)
	rm.RegisterRouter(ctx, "POST", "/musictag/api/scrape/stop", p.scraperHandler.HandleStopScrape, true)
	rm.RegisterRouter(ctx, "POST", "/musictag/api/scrape/retry-failed", p.scraperHandler.HandleRetryFailed, true)

	slog.Info("音乐标签刮削插件路由注册完成")
	return &emptypb.Empty{}, nil
}

// Deinit 反初始化插件
func (p *Plugin) Deinit(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("正在反初始化音乐标签刮削插件")

	if p.scraperManager != nil {
		p.scraperManager.Close()
	}

	return &emptypb.Empty{}, nil
}
