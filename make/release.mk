.PHONY: release
release:
	@echo "Latest tags:"; \
	git tag --sort=-version:refname | head -n 10; \
	echo ""; \
	read -p "Enter version (e.g., v1.0.0): " VERSION; \
	if [ -z "$$VERSION" ]; then \
		echo "Error: Version cannot be empty"; \
		exit 1; \
	fi; \
	git tag -d "$$VERSION" > /dev/null 2>&1 || true; \
	git push --delete origin "$$VERSION" > /dev/null 2>&1 || true; \
	git tag -a "$$VERSION" -m "$$VERSION"; \
	git push origin "$$VERSION"
