.PHONY: build test lint clean dev

# Engine shortcuts
build:
	$(MAKE) -C packages/engine build

test:
	$(MAKE) -C packages/engine test

lint:
	$(MAKE) -C packages/engine lint

clean:
	$(MAKE) -C packages/engine clean

dev:
	$(MAKE) -C packages/engine dev

# Site
site-dev:
	cd packages/site && npm run dev

site-build:
	cd packages/site && npm run build
