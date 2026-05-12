.PHONY: run install-nlp

VENV := .venv
PYTHON := $(VENV)/bin/python3
PIP := $(VENV)/bin/pip3

$(VENV):
	python3 -m venv $(VENV)

install-nlp: $(VENV)
	$(PIP) install -q -r requirements_nlp.txt

run: install-nlp
	@echo "Starting NLP service..."
	@$(PYTHON) nlp_service.py & echo $$! > .nlp.pid
	@echo "Waiting for NLP service to be ready..."
	@for i in $$(seq 1 30); do \
		curl -sf http://127.0.0.1:5001/analyze -X POST \
			-H "Content-Type: application/json" -d '["test"]' > /dev/null 2>&1 && break; \
		sleep 2; \
	done
	@echo "Starting Go server..."
	go run . ; \
	kill $$(cat .nlp.pid) 2>/dev/null; \
	rm -f .nlp.pid
