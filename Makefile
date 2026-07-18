# Garlic Project Makefile

# 配置变量
BINARY_NAME = garlic
GO = go
NPM = npm
PYTHON = python

# Chrome 和 ChromeDriver 下载配置
CHROME_VERSION = 146.0.7645.0
WEBEROER_TOOLS_DIR = tools/webrowser

# 平台检测
ifeq ($(OS),Windows_NT)
    EXT = .exe
    PLATFORM = win64
    UNZIP_CMD = powershell -Command "Expand-Archive -Force"
    CURL_CMD = curl -L
else
    UNAME_S := $(shell uname -s)
    UNAME_M := $(shell uname -m)
    
    ifeq ($(UNAME_S),Darwin)
        ifeq ($(UNAME_M),arm64)
            PLATFORM = mac-arm64
        else
            PLATFORM = mac-x64
        endif
    else
        ifeq ($(UNAME_M),aarch64)
            PLATFORM = linux-arm64
        else
            PLATFORM = linux-x64
        endif
    endif
    
    EXT =
    UNZIP_CMD = unzip -o
    CURL_CMD = curl -L
endif

CHROME_URL = https://cdn.npmmirror.com/binaries/chrome-for-testing/$(CHROME_VERSION)/$(PLATFORM)/chrome-$(PLATFORM).zip
CHROMEDRIVER_URL = https://cdn.npmmirror.com/binaries/chrome-for-testing/$(CHROME_VERSION)/$(PLATFORM)/chromedriver-$(PLATFORM).zip

TARGET = $(BINARY_NAME)$(EXT)

# webrowser 工具独立的 Python 虚拟环境
WEBEROER_VENV = $(WEBEROER_TOOLS_DIR)/.venv
ifeq ($(OS),Windows_NT)
    WEBEROER_VENV_PYTHON = $(WEBEROER_VENV)/Scripts/python.exe
else
    WEBEROER_VENV_PYTHON = $(WEBEROER_VENV)/bin/python
endif

# 默认目标
.PHONY: all
all: setup

# 下载并解压 Chrome 和 ChromeDriver
.PHONY: setup
setup: download-chrome download-chromedriver setup-webrowser build

# 为 webrowser 工具创建独立 venv 并安装依赖
.PHONY: setup-webrowser
setup-webrowser:
	@if [ -f "$(WEBEROER_VENV_PYTHON)" ]; then \
		echo "webrowser venv 已存在，跳过创建"; \
	else \
		echo "正在为 webrowser 创建 venv..."; \
		python3 -m venv "$(WEBEROER_VENV)"; \
		echo "venv 创建完成: $(WEBEROER_VENV)"; \
	fi
	@echo "正在安装 webrowser 依赖..."; \
	"$(WEBEROER_VENV_PYTHON)" -m pip install --upgrade pip; \
	"$(WEBEROER_VENV_PYTHON)" -m pip install -r "$(WEBEROER_TOOLS_DIR)/requirements.txt"; \
	echo "webrowser 依赖安装完成"

.PHONY: download-chrome
download-chrome:
	@if [ -d "$(WEBEROER_TOOLS_DIR)/chrome-$(PLATFORM)" ]; then \
		echo "Chrome 已存在，跳过下载"; \
	else \
		mkdir -p "$(WEBEROER_TOOLS_DIR)/chrome-$(PLATFORM)"; \
		echo "正在下载 Chrome..."; \
		$(CURL_CMD) "$(CHROME_URL)" -o "chrome-$(PLATFORM).zip"; \
		echo "正在解压 Chrome..."; \
		$(UNZIP_CMD) "chrome-$(PLATFORM).zip" -d "$(WEBEROER_TOOLS_DIR)"; \
		rm -f "chrome-$(PLATFORM).zip"; \
		echo "Chrome 下载完成"; \
	fi

.PHONY: download-chromedriver
download-chromedriver:
	@if [ -d "$(WEBEROER_TOOLS_DIR)/chromedriver-$(PLATFORM)" ]; then \
		echo "ChromeDriver 已存在，跳过下载"; \
	else \
		mkdir -p "$(WEBEROER_TOOLS_DIR)/chromedriver-$(PLATFORM)"; \
		echo "正在下载 ChromeDriver..."; \
		$(CURL_CMD) "$(CHROMEDRIVER_URL)" -o "chromedriver-$(PLATFORM).zip"; \
		echo "正在解压 ChromeDriver..."; \
		$(UNZIP_CMD) "chromedriver-$(PLATFORM).zip" -d "$(WEBEROER_TOOLS_DIR)"; \
		rm -f "chromedriver-$(PLATFORM).zip"; \
		echo "ChromeDriver 下载完成"; \
	fi

# 构建前端
.PHONY: build-frontend
build-frontend:
	@echo "正在构建前端..."; \
	cd web && $(NPM) install && $(NPM) run build; \
	echo "前端构建完成"

# 构建后端
.PHONY: build-backend
build-backend:
	@echo "正在构建后端..."; \
	$(GO) build -o $(TARGET) cmd/main.go; \
	echo "后端构建完成: $(TARGET)"

# 构建全部（前端 + 后端）
.PHONY: build
build: build-frontend build-backend
	@echo 构建完成！

# 清理构建产物
.PHONY: clean
clean:
	@echo 正在清理...
	@if [ -f "$(TARGET)" ]; then rm "$(TARGET)"; fi
	@if [ -d "web/dist" ]; then rm -rf web/dist; fi
	@rm -f chrome-*.zip chromedriver-*.zip
	@if [ -d "$(WEBEROER_VENV)" ]; then rm -rf "$(WEBEROER_VENV)"; fi
	@echo 清理完成

# 运行项目
.PHONY: run
run: build
	@echo 正在运行 $(TARGET)...
	@./$(TARGET)

# 帮助信息
.PHONY: help
help:
	@echo Garlic Project Makefile
	@echo 可用目标:
	@echo   setup            - 下载，解压依赖文件 Chrome 和 ChromeDriver，创建 webrowser venv 并完成前后端构建
	@echo   setup-webrowser  - 为 webrowser 工具创建独立 venv 并安装依赖
	@echo   build-frontend   - 构建前端项目
	@echo   build-backend    - 构建后端项目
	@echo   build            - 构建前端和后端（默认）
	@echo   all              - 下载依赖并构建全部
	@echo   clean            - 清理构建产物
	@echo   run              - 构建并运行项目
	@echo   help             - 显示此帮助信息
