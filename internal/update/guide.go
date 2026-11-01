package update

import "fmt"

// 容器环境无法在页面里代用户执行命令，所以把"下一步敲什么"算成结构化数据，
// 前端只负责渲染与复制。版本号/包名/链接一律在此拼好，非法版本号直接不生成引导。
const (
	repoURL     = "https://github.com/busfly/frpc-mesh"
	dockerAsset = "frpc-mesh-v%s-docker-linux-arm64.tar.gz"
	// inspectMounts 在宿主机侧打印真实挂载，用户据此抄出自己的数据目录——容器内看不到宿主机路径
	inspectMounts = `docker inspect frpc-mesh --format '{{ range .Mounts }}-v {{ .Source }}:{{ .Destination }} {{ end }}'`
)

// GuideStep 是一步引导：标题 + 一句大白话说明 + 可复制命令 + 可选链接
type GuideStep struct {
	Title    string     `json:"title"`
	Desc     string     `json:"desc,omitempty"`
	Commands []string   `json:"commands,omitempty"`
	Link     *GuideLink `json:"link,omitempty"`
	Optional bool       `json:"optional,omitempty"`
}

// GuideLink 指向下载直链；URL 以 # 开头表示页内动作（如滚动到备份卡片）
type GuideLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// UpgradeGuide 容器环境的完整升级引导
type UpgradeGuide struct {
	Kind     string      `json:"kind"` // docker_image | source_build
	Intro    string      `json:"intro"`
	Steps    []GuideStep `json:"steps"`
	Fallback GuideStep   `json:"fallback"`
	Note     string      `json:"note"`
}

// buildGuide 按目标版本与容器架构生成引导；latestVer 非 x.y.z 时返回 nil，
// 宁可没有引导也不把来路不明的字符串拼进可复制命令
func buildGuide(latestVer, arch string) *UpgradeGuide {
	if !versionRe.MatchString(latestVer) {
		return nil
	}
	g := &UpgradeGuide{
		Kind:  "source_build",
		Intro: fmt.Sprintf("升级要换一次镜像并重建容器，全程在你自己的电脑终端里执行几条命令，约 2 分钟；期间隧道会断几秒后自动恢复。本页面不能替你执行，只能把命令给你抄。命令里的容器名 frpc-mesh 按你自己的改，不确定就先执行 docker ps 看 NAME 列。本次升级到 v%s。", latestVer),
		Note:  "配置和已下载的 frpc 版本都在宿主机目录里，重建容器不会丢；隧道会中断几秒后自动恢复。",
		Fallback: GuideStep{
			Title:    "万一升级失败或页面打不开怎么办",
			Desc:     "先看报错：下面这条会打印容器最近 100 行输出。容器起不来也不会吃掉配置，因为数据在宿主机目录里。要退回旧版本，重新下载旧版本镜像包 load 回来，再走一次「重建容器」那步即可。",
			Commands: []string{"docker logs --tail 100 frpc-mesh"},
		},
	}
	if arch == "arm64" {
		asset := fmt.Sprintf(dockerAsset, latestVer)
		g.Kind = "docker_image"
		g.Steps = []GuideStep{
			{
				Title:    "（可选）先导出备份",
				Desc:     "配置本来就在宿主机目录里，重建不会动它；想更保险可以先存一份 JSON。",
				Link:     &GuideLink{Label: "去导出备份", URL: "#backup"},
				Optional: true,
			},
			{
				Title: "下载新镜像包",
				Desc:  "约 16MB，下载后不用解压。找不到链接可以从本页「关于」卡片进仓库的 Releases 页。",
				Link:  &GuideLink{Label: asset, URL: fmt.Sprintf("%s/releases/download/v%s/%s", repoURL, latestVer, asset)},
			},
			{
				Title:    "在宿主机终端导入镜像",
				Desc:     "先 cd 到刚才的下载目录（浏览器默认一般是 ~/Downloads），再执行下面这条。",
				Commands: []string{"cd ~/Downloads", fmt.Sprintf("gunzip -c %s | docker load", asset)},
			},
			{
				Title: "重建容器（二选一）",
				Desc:  "判断方法：你放 frpc-mesh 的文件夹里有 docker-compose.yml 就用「用 compose」那两条；没有就用「用 docker run」那三条——先跑 inspect 那条，把输出里 `-v /宿主机路径:/data` 的路径抄到最后一条对应位置。",
				Commands: []string{
					"# 用 compose：cd 到你放 docker-compose.yml 的目录后执行",
					"docker compose up -d --force-recreate",
					"# 用 docker run：先抄出自己的数据目录，再删旧容器并用新镜像起一个",
					inspectMounts,
					"docker rm -f frpc-mesh",
					"docker run -d --name frpc-mesh --restart unless-stopped --add-host host.docker.internal:host-gateway -e TZ=Asia/Shanghai -p 18080:18080 -v 抄到的数据目录路径:/data frpc-mesh:latest",
				},
			},
			{
				Title: "回页面确认",
				Desc:  fmt.Sprintf("等 10 秒刷新本页面，顶栏版本变成 v%s 就成功了。", latestVer),
			},
		}
		return g
	}
	g.Steps = []GuideStep{
		{
			Title:    "（可选）先导出备份",
			Desc:     "配置本来就在宿主机目录里，重建不会动它；想更保险可以先存一份 JSON。",
			Link:     &GuideLink{Label: "去导出备份", URL: "#backup"},
			Optional: true,
		},
		{
			Title:    "更新源码并重新构建镜像",
			Desc:     fmt.Sprintf("当前版本没有发布本平台（linux/%s）的 Docker 镜像包，所以在放 frpc-mesh 源码的目录里构建：先拉最新代码，再带 --build 重建。", arch),
			Commands: []string{"cd 你放 frpc-mesh 源码的目录", "git pull", "docker compose up -d --build --force-recreate"},
		},
		{
			Title: "回页面确认",
			Desc:  fmt.Sprintf("等构建结束（约 1-2 分钟）后刷新本页面，顶栏版本变成 v%s 就成功了。", latestVer),
		},
	}
	return g
}
