##@ Run

run: build/$(APP_NAME) ## Run  app
	$<
dev-run: | runtime/tools/go ## Run dev server. If detect file change, automatically rebuild&restart server
	go tool watcher \
		--include "go.mod" \
		--include "go.sum" \
		--include "**.go" \
		--include "Makefile" \
		--include "scripts/makefile.d/*.mk" \
		--include "runtime/config.yaml" \
		--exclude "build/**" \
		--exclude "**.sw*" \
		-- \
	$(MAKE) test run

reset: ## Kill all make process. Use when dev-run stuck.
	ps -e | grep $(APP_NAME) | grep -v grep | awk '{print $$1}' | xargs kill

.PHONY: run dev-run reset
