package update

import (
	"strings"
	"testing"
)

func TestBuildGuideArm64GivesImageSteps(t *testing.T) {
	g := buildGuide("1.1.1", "arm64")
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
}

func TestBuildGuideOtherArchUsesSourceBuild(t *testing.T) {
	g := buildGuide("1.1.1", "amd64")
	if g == nil {
		t.Fatal("amd64 容器也应生成引导（源码构建）")
	}
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
		t.Error("非 arm64 不能给出镜像包下载链接（该包不存在）")
	}
	if !strings.Contains(txt.String(), "--build") {
		t.Error("源码构建步骤应含 --build")
	}
}

func TestBuildGuideRejectsBadVersion(t *testing.T) {
	for _, v := range []string{"", "1.1", "v1.1.1", "1.1.1; rm -rf /", "1.1.1 $(x)"} {
		if g := buildGuide(v, "arm64"); g != nil {
			t.Errorf("非法版本号 %q 不应生成引导", v)
		}
	}
}

func TestCapabilityForContainerCarriesGuide(t *testing.T) {
	u := NewUpgrader(New(newStore(t), "1.0.0"), &fakeStop{}, t.TempDir())
	defer func() { inContainer = defaultInContainer }()
	inContainer = func() bool { return true }
	cap := u.CapabilityFor("1.1.1", []Asset{{Name: checksums}})
	if cap.Mode != "container" || cap.Guide == nil {
		t.Fatalf("容器环境应带 guide: %+v", cap)
	}
}
