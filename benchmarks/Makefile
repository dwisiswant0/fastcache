OUT := out
HEADER := $(OUT)/header
RESULTS := $(OUT)/results
Fastcache := $(OUT)/fastcache
FastcacheFork := $(OUT)/fastcache_fork
Otter := $(OUT)/otter

bench:
	@mkdir -p $(OUT) 2>/dev/null || true
	@rm -f $(OUT)/*

	@go test -count=10 -bench=. -benchmem | tee $(RESULTS)
	@grep -E '^(goos|goarch|pkg|cpu):' $(RESULTS) > $(HEADER)

	@cat $(HEADER) > $(Fastcache)
	@grep '^BenchmarkFastcache/' $(RESULTS) | sed 's/BenchmarkFastcache\//Benchmark/' >> $(Fastcache)
	@echo "PASS" >> $(Fastcache)

	@cat $(HEADER) > $(FastcacheFork)
	@grep '^BenchmarkFastcacheFork/' $(RESULTS) | sed 's/BenchmarkFastcacheFork\//Benchmark/' >> $(FastcacheFork)
	@echo "PASS" >> $(FastcacheFork)

	@cat $(HEADER) > $(Otter)
	@grep '^BenchmarkOtter/' $(RESULTS) | sed 's/BenchmarkOtter\//Benchmark/' >> $(Otter)
	@echo "PASS" >> $(Otter)
