.PHONY: install dev test e2e clean

install:
	pip install -e .

dev:
	pip install -e ".[dev]"

test:
	pytest tests/ -v

e2e:
	@echo "Make sure sandbox is running: cd ../sandbox && docker-compose up -d"
	pytest tests/test_e2e.py -v

clean:
	rm -rf build/ dist/ *.egg-info/ __pycache__/ .pytest_cache/
	find . -name "*.pyc" -delete

