package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"yantai/frontend"
	"yantai/internal/workspace"
)

type App struct {
	ctx               context.Context
	base              string
	token             string
	dirty             atomic.Bool
	smokeMarker       string
	connectionPath    string
	connectionWritten bool
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	connection, _ := json.MarshalIndent(a.APIInfo(), "", "  ")
	if err := os.WriteFile(a.connectionPath, connection, 0600); err != nil {
		log.Print(err)
	} else {
		a.connectionWritten = true
	}
}
func (a *App) APIInfo() map[string]string { return map[string]string{"base": a.base, "token": a.token} }
func (a *App) SetDirty(value bool)        { a.dirty.Store(value) }
func (a *App) MarkReady() {
	if a.smokeMarker != "" {
		if err := os.WriteFile(a.smokeMarker, []byte("Desktop frontend and authenticated API initialized\n"), 0600); err != nil {
			log.Print(err)
		}
		go func() { time.Sleep(300 * time.Millisecond); runtime.Quit(a.ctx) }()
	}
}
func (a *App) BeforeClose(ctx context.Context) bool {
	if !a.dirty.Load() {
		return false
	}
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.WarningDialog, Title: "还有未保存的修改", Message: "请等待自动保存完成。如果保存失败，可以导出当前草稿。关闭后，下次打开项目可恢复本机草稿缓存。", Buttons: []string{"继续写作", "关闭窗口"}, DefaultButton: "继续写作", CancelButton: "继续写作"})
	return err != nil || answer != "关闭窗口"
}
func (a *App) SaveText(name, text string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "导出小说", DefaultFilename: filepath.Base(name)})
	if err != nil || path == "" {
		return path, err
	}
	err = os.WriteFile(path, []byte(text), 0600)
	return path, err
}
func (a *App) SaveBinary(name, encoded string) (string, error) {
	if len(encoded) > 64<<20 {
		return "", errors.New("导出文件过大")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "导出小说", DefaultFilename: filepath.Base(name)})
	if err != nil || path == "" {
		return path, err
	}
	return path, os.WriteFile(path, data, 0600)
}
func (a *App) OpenExternal(address string) {
	if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "http://") {
		runtime.BrowserOpenURL(a.ctx, address)
	}
}
func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	root := os.Getenv("YANTAI_DATA")
	if root == "" {
		root = os.Getenv("NOVEL_EDIT_DATA")
	}
	if root == "" {
		root = filepath.Join(filepath.Dir(exe), "data")
	}
	store, err := workspace.NewStore(root)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	a := &App{base: "http://" + listener.Addr().String(), token: workspace.ID()}
	a.connectionPath = filepath.Join(root, "agent-connection.json")
	defer func() {
		if a.connectionWritten {
			os.Remove(a.connectionPath)
		}
	}()
	smoke := os.Getenv("YANTAI_SMOKE") == "1" || os.Getenv("NOVEL_EDIT_SMOKE") == "1"
	instanceID := "f13ef385-92fb-4a6d-beb9-f3e6c4cbabde"
	if smoke {
		instanceID = "8d6a27b4-fcbd-486d-b3cf-184ad3ee33bf"
		a.smokeMarker = filepath.Join(root, "smoke-ready.txt")
	}
	svc := &workspace.Server{Store: store, Token: a.token}
	httpServer := &http.Server{Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	defer httpServer.Close()
	err = wails.Run(&options.App{Title: "Yantai · 小说写作工作台", Width: 1500, Height: 960, MinWidth: 1050, MinHeight: 700, StartHidden: smoke, BackgroundColour: &options.RGBA{R: 245, G: 243, B: 237, A: 255}, AssetServer: &assetserver.Options{Assets: frontend.Assets()}, OnStartup: a.Startup, OnBeforeClose: a.BeforeClose, Bind: []interface{}{a}, SingleInstanceLock: &options.SingleInstanceLock{UniqueId: instanceID}, Windows: &windows.Options{WebviewIsTransparent: false}, EnableDefaultContextMenu: true})
	if err != nil {
		log.Fatal(err)
	}
}
