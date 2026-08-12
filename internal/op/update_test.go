package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func initTestDB(t *testing.T, models ...any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	if err := db.InitDB("sqlite", path, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.GetDB().AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	return path
}

// 回归测试：partialUpdate 重构后 ChannelUpdate 的字段映射、
// serializer 字段（base_urls / custom_header）与 recover 语义。

func TestChannelUpdatePartialFields(t *testing.T) {
	initTestDB(t, &model.Channel{}, &model.ChannelKey{})
	if err := channelRefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	ch := model.Channel{Name: "orig", Type: "openai", Model: "a,b", Enabled: true}
	if err := db.GetDB().Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	channelCache.Set(ch.ID, ch)

	name := "renamed"
	proxy := "http://proxy:8080"
	override := `{"temperature": 0.5}`
	cooldown := 99
	updated, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID:                   ch.ID,
		Name:                 &name,
		ChannelProxy:         &proxy,
		ParamOverride:        &override,
		RateLimitCooldownSec: &cooldown,
	}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.ChannelProxy == nil || *updated.ChannelProxy != proxy ||
		updated.ParamOverride == nil || *updated.ParamOverride != override ||
		updated.RateLimitCooldownSec == nil || *updated.RateLimitCooldownSec != cooldown {
		t.Fatalf("partial update failed: %+v", updated)
	}
	// 未传字段保持不变
	if updated.Model != "a,b" || !updated.Enabled {
		t.Fatalf("untouched fields changed: model=%q enabled=%v", updated.Model, updated.Enabled)
	}

	// ClearRateLimitCooldown → NULL
	updated2, err := ChannelUpdate(&model.ChannelUpdateRequest{ID: ch.ID, ClearRateLimitCooldown: true}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated2.RateLimitCooldownSec != nil {
		t.Fatalf("clear cooldown: got %v want nil", *updated2.RateLimitCooldownSec)
	}
}

func TestChannelUpdateSerializerFields(t *testing.T) {
	initTestDB(t, &model.Channel{}, &model.ChannelKey{})
	if err := channelRefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	ch := model.Channel{Name: "s1", Type: "openai"}
	if err := db.GetDB().Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	channelCache.Set(ch.ID, ch)

	baseUrls := []model.BaseUrl{{URL: "https://a.example.com", Delay: 10}}
	customHeader := []model.CustomHeader{{HeaderKey: "X-Test", HeaderValue: "v1"}}
	updated, err := ChannelUpdate(&model.ChannelUpdateRequest{
		ID:           ch.ID,
		BaseUrls:     &baseUrls,
		CustomHeader: &customHeader,
	}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.BaseUrls) != 1 || updated.BaseUrls[0].URL != "https://a.example.com" {
		t.Fatalf("base_urls: got %+v", updated.BaseUrls)
	}
	if len(updated.CustomHeader) != 1 || updated.CustomHeader[0].HeaderKey != "X-Test" {
		t.Fatalf("custom_header: got %+v", updated.CustomHeader)
	}
	// 数据库持久化一致（serializer 编码与读取对称）
	var persisted model.Channel
	if err := db.GetDB().First(&persisted, ch.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(persisted.BaseUrls) != 1 || persisted.BaseUrls[0].URL != "https://a.example.com" {
		t.Fatalf("db base_urls: got %+v", persisted.BaseUrls)
	}
}

func TestChannelUpdateEmptyRequest(t *testing.T) {
	initTestDB(t, &model.Channel{}, &model.ChannelKey{})
	if err := channelRefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	ch := model.Channel{Name: "n1", Type: "openai"}
	if err := db.GetDB().Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	channelCache.Set(ch.ID, ch)
	if _, err := ChannelUpdate(&model.ChannelUpdateRequest{ID: ch.ID}, context.Background()); err != nil {
		t.Fatalf("empty update should succeed: %v", err)
	}
}

func TestGroupUpdatePartialFields(t *testing.T) {
	initTestDB(t, &model.Group{}, &model.GroupItem{})
	if err := groupRefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	g := model.Group{Name: "g1", Mode: model.GroupModeFailover}
	if err := db.GetDB().Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	groupCache.Set(g.ID, g)
	groupMap.Set(g.Name, g)

	name := "g1-renamed"
	timeout := 30
	updated, err := GroupUpdate(&model.GroupUpdateRequest{
		ID:                g.ID,
		Name:              &name,
		FirstTokenTimeOut: &timeout,
	}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.FirstTokenTimeOut != timeout {
		t.Fatalf("group partial update failed: %+v", updated)
	}
	if updated.Mode != model.GroupModeFailover {
		t.Fatalf("untouched mode changed: %v", updated.Mode)
	}
}
