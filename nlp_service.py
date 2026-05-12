#!/usr/bin/env python3
"""
Local NLP microservice — translation (offline) + sentiment (VADER).
No external API calls. The translation model is downloaded once and cached by argostranslate.

Usage:
    pip install -r requirements_nlp.txt
    python nlp_service.py
"""

import json
import logging
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Translation — argostranslate (offline)
# ---------------------------------------------------------------------------
def load_translator():
    import argostranslate.package
    import argostranslate.translate

    installed = {
        (p.from_code, p.to_code)
        for p in argostranslate.package.get_installed_packages()
    }
    if ("en", "fr") not in installed:
        log.info("Downloading en→fr language pack…")
        argostranslate.package.update_package_index()
        available = argostranslate.package.get_available_packages()
        pkg = next(p for p in available if p.from_code == "en" and p.to_code == "fr")
        argostranslate.package.install_from_path(pkg.download())
        log.info("en→fr pack installed.")

    def translate(text: str) -> str:
        return argostranslate.translate.translate(text, "en", "fr")

    return translate


# ---------------------------------------------------------------------------
# Sentiment — VADER (social media optimised, pure Python, no torch)
# ---------------------------------------------------------------------------
def load_sentiment():
    from vaderSentiment.vaderSentiment import SentimentIntensityAnalyzer

    analyzer = SentimentIntensityAnalyzer()
    log.info("VADER sentiment ready.")

    def analyze(text: str) -> str:
        compound = analyzer.polarity_scores(text)["compound"]
        if compound >= 0.05:
            return "bullish"
        if compound <= -0.05:
            return "bearish"
        return "neutral"

    return analyze


# ---------------------------------------------------------------------------
# HTTP server
# ---------------------------------------------------------------------------
class Handler(BaseHTTPRequestHandler):
    translate = None
    sentiment = None

    def log_message(self, fmt, *args):
        pass  # silence default access log

    def do_POST(self):
        if self.path != "/analyze":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        try:
            titles = json.loads(body)
            if not isinstance(titles, list):
                raise ValueError("expected JSON array")
        except Exception as e:
            self.send_error(400, str(e))
            return

        results = []
        for title in titles:
            try:
                translation = Handler.translate(title) if title.strip() else ""
                sent = Handler.sentiment(title) if title.strip() else "neutral"
            except Exception as e:
                log.warning("Error processing title %r: %s", title, e)
                translation, sent = "", "neutral"
            results.append({"translation": translation, "sentiment": sent})

        payload = json.dumps(results).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", len(payload))
        self.end_headers()
        self.wfile.write(payload)


def main():
    port = 5001
    log.info("Initializing models…")
    Handler.translate = load_translator()
    Handler.sentiment = load_sentiment()
    log.info("NLP service listening on http://localhost:%d", port)
    server = HTTPServer(("127.0.0.1", port), Handler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        log.info("Stopped.")
        sys.exit(0)


if __name__ == "__main__":
    main()
