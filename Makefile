Name = fkteams
Version = 0.0.1
BuildTime = $(shell date +'%Y-%m-%d %H:%M:%S')

# 提取当前系统的 OS 和 ARCH
CURRENT_OS = $(shell go env GOOS)
CURRENT_ARCH = $(shell go env GOARCH)

LDFlags = -ldflags "-s -w -X '${Name}/version.version=$(Version)' -X '${Name}/version.buildTime=${BuildTime}'"

# 默认全量编译的目标列表
targets ?= darwin:arm64 windows:amd64 linux:amd64

.DEFAULT_GOAL := native

# 1. 原生系统编译：强制指定输出格式为 fkteams_os_arch
native:
	@$(MAKE) build t="$(CURRENT_OS):$(CURRENT_ARCH)"

# 2. 编译所有预设平台
all:
	@$(MAKE) build t="$(targets)"

# 3. 核心编译逻辑
build:
	@if [ -z "$(t)" ]; then \
		echo "错误: 请指定目标，例如 make build t=linux:amd64"; \
		exit 1; \
	fi
	@$(foreach n, $(t),\
		os=$$(echo "$(n)" | cut -d : -f 1);\
		arch=$$(echo "$(n)" | cut -d : -f 2);\
		suffix=""; \
		if [ "$${os}" = "windows" ]; then suffix=".exe"; fi; \
		output_name="./release/${Name}_$${os}_$${arch}$${suffix}"; \
		echo "正在编译: $${os}/$${arch}..."; \
		env CGO_ENABLED=0 GOOS=$${os} GOARCH=$${arch} go build -trimpath $(LDFlags) -o $${output_name} ./main.go;\
		echo "编译完成: $${output_name}";\
	)

clean:
	rm -rf ./release

.PHONY: native all build clean