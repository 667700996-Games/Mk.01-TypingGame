PYTHON ?= python3
.PHONY: build package test run clean test-cleanup
build package test run clean:
	$(PYTHON) -B scripts/build.py $@
test-cleanup:
	$(PYTHON) -B -m unittest discover -s scripts -p 'test_*.py' -v
