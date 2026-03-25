PLUGIN_NAME=musictag

.PHONY: build clean

build:
	@echo "Building $(PLUGIN_NAME) plugin..."
	GOOS=wasip1 GOARCH=wasm go build -o $(PLUGIN_NAME).wasm -buildmode=c-shared .
	@echo "Build complete: $(PLUGIN_NAME).wasm"

clean:
	@echo "Cleaning build artifacts..."
	rm -f $(PLUGIN_NAME).wasm
	@echo "Clean complete"

info:
	@echo "Plugin: $(PLUGIN_NAME)"
	@echo "Output: $(PLUGIN_NAME).wasm"

all: build info ## 完整构建流程
