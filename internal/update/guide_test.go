package update

import (
	"strings"
	"testing"
)

// arm64 镜像包资产名，构造测试用 assets 时复用
func dockerAssets(ver string) []Asset {
	return []Asset{{Name: dockerAssetName(ver)}, {Name: checksums}}
}

func TestBuildGuideArm64GivesImageSteps(t *testing.T) {
	g := buildGuide("1.1.1", "arm64", dockerAssets("1.1.1"))
	if g == nil {
		t.Fatal("arm64 容器应生成引导")
	}
	if g.Kind != "docker_image" {
		t.Errorf("Kind = %s, want docker_image", g.Kind)
	}
	var all strings.Builder
	for _, s := range g.Steps {
		all.WriteString(s.Title)
		all.WriteString(strings.Join(s.Commands, "\n"))
		if s.Link != nil {
			all.WriteString(s.Link.URL)
		}
	}
	txt := all.String()
	for _, want := range []string{
		"frpc-mesh-v1.1.1-docker-linux-arm64.tar.gz",
		"releases/download/v1.1.1/frpc-mesh-v1.1.1-docker-linux-arm64.tar.gz",
		"docker load",
		"docker compose up -d --force-recreate",
		"docker inspect frpc-mesh",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("引导缺少 %q", want)
		}
	}
	if g.Fallback.Title == "" || len(g.Fallback.Commands) == 0 {
		t.Error("应带折叠的回退指引")
	}
	if !strings.Contains(g.Intro, "本次升级到 v1.1.1") {
		t.Errorf("intro 要说清升级目标，实为 %q", g.Intro)
	}
	if strings.Contains(g.Intro, "当前版本 v1.1.1") {
		t.Error("intro 不能把最新版本说成当前版本")
	}
}

// compose 与 docker run 两套命令各由一行 # 说明开头，前端据此渲染分组、不给复制按钮
func TestBuildGuideRebuildStepSeparatesTwoCommandSets(t *testing.T) {
	g := buildGuide("1.1.1", "arm64", dockerAssets("1.1.1"))
	var rebuild *GuideStep
	for i := range g.Steps {
		if strings.Contains(g.Steps[i].Title, "重建容器") {
			rebuild = &g.Steps[i]
		}
	}
	if rebuild == nil {
		t.Fatal("缺少重建容器步骤")
	}
	var captions []string
	for _, c := range rebuild.Commands {
		if strings.HasPrefix(strings.TrimSpace(c), "#") {
			captions = append(captions, c)
		}
	}
	if len(captions) != 2 {
		t.Fatalf("重建步骤应有 compose / docker run 两行 # 说明，实为 %d 行: %v", len(captions), captions)
	}
	for _, want := range []string{"compose", "docker run"} {
		found := false
		for _, c := range captions {
			if strings.Contains(c, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("说明行缺少 %q 一套：%v", want, captions)
		}
	}
}

func TestBuildGuideOtherArchUsesSourceBuild(t *testing.T) {
	g := buildGuide("1.1.1", "amd64", dockerAssets("1.1.1"))
	if g == nil {
		t.Fatal("amd64 容器也应生成引导（源码构建）")
	}
	assertSourceBuild(t, g)
}

// 该版本漏发镜像包时，arm64 容器也不能拿到 404 下载链接
func TestBuildGuideArm64FallsBackWhenImageMissing(t *testing.T) {
	g := buildGuide("1.1.1", "arm64", []Asset{{Name: checksums}})
	if g == nil {
		t.Fatal("应生成引导")
	}
	assertSourceBuild(t, g)
}

func assertSourceBuild(t *testing.T, g *UpgradeGuide) {
	t.Helper()
	if g.Kind != "source_build" {
		t.Errorf("Kind = %s, want source_build", g.Kind)
	}
	var txt strings.Builder
	for _, s := range g.Steps {
		txt.WriteString(strings.Join(s.Commands, "\n"))
		if s.Link != nil {
			txt.WriteString(s.Link.URL)
		}
	}
	if strings.Contains(txt.String(), "releases/download") {
		t.Error("源码构建引导不能给镜像包下载链接（该包不存在）")
	}
	if !strings.Contains(txt.String(), "--build") {
		t.Error("源码构建步骤应含 --build")
	}
}

func TestBuildGuideRejectsBadVersion(t *testing.T) {
	for _, v := range []string{"", "1.1", "v1.1.1", "1.1.1; rm -rf /", "1.1.1 $(x)", "1.1.1\nrm -rf /"} {
		if g := buildGuide(v, "arm64", dockerAssets(v)); g != nil {
			t.Errorf("非法版本号 %q 不应生成引导", v)
		}
	}
}

func TestCapabilityForContainerCarriesGuide(t *testing.T) {
	u := NewUpgrader(New(newStore(t), "1.0.0"), &fakeStop{}, t.TempDir())
	defer func() { inContainer = defaultInContainer }()
	inContainer = func() bool { return true }
	cap := u.CapabilityFor("1.1.1", dockerAssets("1.1.1"))
	if cap.Mode != "container" || cap.Guide == nil {
		t.Fatalf("容器环境应带 guide: %+v", cap)
	}
}

// 已是最新（或领先于最新）时不能再给"升级步骤"，否则等于引导用户往回装旧版本
func TestCapabilityForContainerNoGuideWhenUpToDate(t *testing.T) {
	defer func() { inContainer = defaultInContainer }()
	inContainer = func() bool { return true }
	cases := []struct{ current, latest string }{
		{"1.1.1", "1.1.1"},
		{"1.1.2", "1.1.1"},
		{"1.0.0", "1.1"},
	}
	for _, c := range cases {
		u := NewUpgrader(New(newStore(t), c.current), &fakeStop{}, t.TempDir())
		cap := u.CapabilityFor(c.latest, dockerAssets(c.latest))
		if cap.Guide != nil {
			t.Errorf("current=%s latest=%s 不该生成升级引导", c.current, c.latest)
		}
	}
}

// 二进制环境走一键升级，不该出现容器引导
func TestCapabilityForBinaryHasNoGuide(t *testing.T) {
	u := NewUpgrader(New(newStore(t), "1.0.0"), &fakeStop{}, t.TempDir())
	defer func() { inContainer = defaultInContainer }()
	inContainer = func() bool { return false }
	assets := append(dockerAssets("1.1.1"), Asset{Name: assetNameFor("1.1.1")})
	cap := u.CapabilityFor("1.1.1", assets)
	if cap.Mode != "binary" || !cap.CanUpgrade {
		t.Fatalf("二进制环境应可就地升级: %+v", cap)
	}
	if cap.Guide != nil {
		t.Error("二进制环境不该带容器引导")
	}
}
