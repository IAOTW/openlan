SHELL := /bin/bash

.ONESHELL:
.PHONY: docker linux linux-rpm darwin darwin-zip windows windows-zip test vendor

## version
LSB = $(shell lsb_release -i -s)$(shell lsb_release -r -s)
VER = $(shell cat VERSION)
ARCH = $(shell uname -m)

## declare directory
SD = $(shell pwd)
BD = "$(SD)/build"
CD = "$(SD)/build/coverage"
LINUX_DIR ?= "openlan-$(LSB)-$(VER).$(ARCH)"
WIN_DIR ?= "openlan-windows-$(VER).$(ARCH)"
MAC_DIR ?= "openlan-darwin-$(VER).$(ARCH)"

## declare flags
MOD = github.com/luscis/openlan/pkg/libol
LDFLAGS += -X $(MOD).Date=$(shell date +%FT%T%z)
LDFLAGS += -X $(MOD).Version=$(VER)

build: test pkg

pkg: clean linux-rpm linux-bin windows-gz darwin-gz ## build all plaftorm packages

gz: linux-gz windows-gz darwin-gz

help: ## show make targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {sub("\\\\n",sprintf("\n%22c"," "), $$2);\
	printf " \033[36m%-20s\033[0m  %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## all platform
bin: linux windows darwin ## build all platform binary

## mkdir -p /opt/openlan
# cp ./dist/rootfs/{var,etc} /opt/openlan
# cp ./docker/docker-compose.yml /opt/openlan
# cd /opt/openlan
## docker-compose up -d
docker: linux-bin docker-rhel docker-deb ## build docker images

docker-rhel:
	cp $(SD)/docker/openlan.centos $(BD)
	cd $(BD) && docker build -t openlan:$(VER).$(ARCH) --build-arg BIN=$(LINUX_DIR).bin -f openlan.centos  .

docker-deb:
	cp $(SD)/docker/openlan.debian $(BD)
	cd $(BD) && docker build -t openlan:d$(VER).$(ARCH) --build-arg BIN=$(LINUX_DIR).bin -f openlan.debian  .

clean: ## clean cache
	rm -rvf ./build

## prepare environment
update:
	git submodule init
	git submodule update

vendor:
	go clean -modcache
	go mod tidy
	go mod vendor -v

env:
	@mkdir -p $(BD)
	@go version
	@gofmt -w -s ./pkg ./cmd

## linux platform
linux: linux-proxy linux-point linux-switch

rpm: env ## build rpm packages
	mkdir -p ~/rpmbuild/SPECS
	mkdir -p ~/rpmbuild/SOURCES
	sed -e "s/Version:.*/Version:\ $(VER)/" $(SD)/dist/openlan.spec.in > ~/rpmbuild/SPECS/openlan.spec
	@dist/spec.sh
	rpmbuild -ba ~/rpmbuild/SPECS/openlan.spec

## compile command line
cmd: env
	go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan ./cmd/main.go

openudp: env
	go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openudp ./cmd/openudp

linux: linux-point linux-switch linux-proxy ## build all linux binary

linux-point: env
	go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-point ./cmd/point_linux

linux-switch: env cmd openudp
	go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-switch ./cmd/switch

linux-proxy: env
	go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-proxy ./cmd/proxy


linux-gz: install ## build linux packages
	@rm -rf $(LINUX_DIR).tar.gz
	tar -cf $(LINUX_DIR).tar $(LINUX_DIR) && mv $(LINUX_DIR).tar $(BD)
	@rm -rf $(LINUX_DIR)
	gzip -f $(BD)/$(LINUX_DIR).tar

linux-bin: linux-gz ## build linux install binary
	@cat $(SD)/dist/rootfs/var/openlan/script/install.sh > $(BD)/$(LINUX_DIR).bin && \
	echo "__ARCHIVE_BELOW__:" >> $(BD)/$(LINUX_DIR).bin && \
	cat $(BD)/$(LINUX_DIR).tar.gz >> $(BD)/$(LINUX_DIR).bin && \
	chmod +x $(BD)/$(LINUX_DIR).bin && \
	echo "Save to $(BD)/$(LINUX_DIR).bin"

install: env linux ## install packages
	@mkdir -p $(LINUX_DIR)
	@cp -rf $(SD)/dist/rootfs/{etc,var,usr} $(LINUX_DIR)
	@mkdir -p $(LINUX_DIR)/var/openlan/{cert,openvpn,point,l2tp,dhcp}
	@cp -rf $(SD)/dist/cert/openlan/cert $(LINUX_DIR)/var/openlan
	@cp -rf $(SD)/dist/cert/openlan/ca/ca.crt $(LINUX_DIR)/var/openlan/cert
	@cp -rf $(SD)/pkg/public $(LINUX_DIR)/var/openlan
	@mkdir -p $(LINUX_DIR)/usr/bin
	@cp -rf $(BD)/{openudp,openlan} $(LINUX_DIR)/usr/bin
	@cp -rf $(BD)/{openlan-point,openlan-proxy,openlan-switch} $(LINUX_DIR)/usr/bin

## cross build for windows
windows: windows-point ## build windows binary

windows-point: env
	GOOS=windows go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-point.exe ./cmd/point_windows
	GOOS=windows go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-proxy.exe ./cmd/proxy

windows-gz: env windows ## build windows packages
	@rm -rf $(WIN_DIR) && mkdir -p $(WIN_DIR)
	@rm -rf $(WIN_DIR).tar.gz

	@cp -rf $(SD)/dist/rootfs/etc/openlan/point.json.example $(WIN_DIR)/point.json
	@cp -rf $(BD)/openlan-point.exe $(WIN_DIR)
	@cp -rf $(SD)/dist/rootfs/etc/openlan/proxy.json.example $(WIN_DIR)/proxy.json
	@cp -rf $(BD)/openlan-proxy.exe $(WIN_DIR)

	tar -cf $(WIN_DIR).tar $(WIN_DIR) && mv $(WIN_DIR).tar $(BD)
	@rm -rf $(WIN_DIR)
	gzip -f $(BD)/$(WIN_DIR).tar

windows-syso: ## build windows syso
	rsrc -manifest ./cmd/point_windows/main.manifest -ico ./cmd/point_windows/main.ico  -o ./cmd/point_windows/main.syso

## cross build for osx
osx: darwin

darwin: env ## build darwin binary
	GOOS=darwin go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(BD)/openlan-point.darwin ./cmd/point_darwin

darwin-gz: env darwin ## build darwin packages
	@rm -rf $(MAC_DIR) && mkdir -p $(MAC_DIR)
	@rm -rf $(MAC_DIR).tar.gz

	@cp -rf $(SD)/dist/rootfs/etc/openlan/point.json.example $(MAC_DIR)/point.json
	@cp -rf $(BD)/openlan-point.darwin $(MAC_DIR)

	tar -cf $(MAC_DIR).tar $(MAC_DIR) && mv $(MAC_DIR).tar $(BD)
	@rm -rf $(MAC_DIR)
	gzip -f $(BD)/$(MAC_DIR).tar

## unit test
test: ## execute unit test
	go clean -testcache
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/access
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/libol
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/models
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/cache
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/config
	go test -v -mod=vendor -bench=. github.com/luscis/openlan/pkg/network

## coverage
cover: env ## execute unit test and output coverage
	@rm -rvf $(CD) && mkdir -p $(CD)
	@go test -mod=vendor github.com/luscis/openlan/pkg/access -coverprofile=$(CD)/0.out -race -covermode=atomic
	@go test -mod=vendor github.com/luscis/openlan/pkg/libol -coverprofile=$(CD)/1.out -race -covermode=atomic
	@go test -mod=vendor github.com/luscis/openlan/pkg/models -coverprofile=$(CD)/2.out -race -covermode=atomic
	@go test -mod=vendor github.com/luscis/openlan/pkg/cache -coverprofile=$(CD)/3.out -race -covermode=atomic
	@go test -mod=vendor github.com/luscis/openlan/pkg/config -coverprofile=$(CD)/4.out -race -covermode=atomic
	@go test -mod=vendor github.com/luscis/openlan/pkg/network -coverprofile=$(CD)/5.out -race -covermode=atomic

	@echo 'mode: atomic' > $(SD)/coverage.out && \
	tail -q -n +2 $(CD)/*.out >> $(SD)/coverage.out
	go tool cover -html=coverage.out -o coverage.html
