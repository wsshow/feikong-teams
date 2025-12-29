Name = fkteams

Version = 0.0.2

BuildTime = $(shell date +'%Y-%m-%d %H:%M:%S')

LDFlags = -ldflags "-s -w -X '${Name}/version.version=$(Version)' -X '${Name}/version.buildTime=${BuildTime}'"

os-archs=darwin:arm64

build: app

app:
	@$(foreach n, $(os-archs),\
		os=$(shell echo "$(n)" | cut -d : -f 1);\
		arch=$(shell echo "$(n)" | cut -d : -f 2);\
		target_suffix=$${os}_$${arch};\
		echo "Build $${os}-$${arch}...";\
		env CGO_ENABLED=0 GOOS=$${os} GOARCH=$${arch} go build -trimpath $(LDFlags) -o ./release/${Name}_$${target_suffix} ./main.go;\
		echo "Build $${os}-$${arch} done";\
	)

clean:
	rm -rf ./release