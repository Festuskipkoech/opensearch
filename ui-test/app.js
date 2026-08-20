const API_URL = "http://localhost:8080";
const MAX_RESULTS = 10;

const input = document.getElementById("search-input");
const btn = document.getElementById("search-btn");
const container = document.getElementById("results-container");
const pills = document.querySelectorAll(".intent-pill");

let activeIntent = "";

pills.forEach((pill) => {
  pill.addEventListener("click", () => {
    pills.forEach((p) => p.classList.remove("active"));
    pill.classList.add("active");
    activeIntent = pill.dataset.intent;
  });
});

btn.addEventListener("click", runSearch);

input.addEventListener("keydown", (e) => {
  if (e.key === "Enter") runSearch();
});

async function runSearch() {
  const query = input.value.trim();
  if (!query) return;

  setLoading(true);

  try {
    const body = { query, max_results: MAX_RESULTS };
    if (activeIntent) body.intent = activeIntent;

    const res = await fetch(`${API_URL}/search`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: "request failed" }));
      renderError(err.error || `error ${res.status}`);
      return;
    }

    const data = await res.json();
    console.log(data);
    renderResults(data);
  } catch (err) {
    renderError("could not reach the search engine");
  } finally {
    setLoading(false);
  }
}

function setLoading(on) {
  btn.disabled = on;
  if (on) {
    container.innerHTML = `<div class="state-message loading-dots">searching</div>`;
  }
}

function renderResults(data) {
  if (!data.results || data.results.length === 0) {
    container.innerHTML = `<div class="state-message">no results found</div>`;
    return;
  }

  const intentLabel = data.uncertain
    ? `${data.intent} · uncertain`
    : data.intent;

  const cachedTag = data.cached ? " · cached" : "";

  const bar = `
    <div class="results-bar">
      <div class="results-count">
        <strong>${data.results.length}</strong> results · intent: ${intentLabel}${cachedTag}
      </div>
      <div class="results-latency">${data.latency_ms}ms</div>
    </div>
  `;

  const cards = data.results.map((r) => resultCard(r)).join("");
  container.innerHTML = bar + cards;
}

function resultCard(r) {
  const scorePct = Math.round((r.Score || 0) * 100);

  const contentBadge = r.Content
    ? `<div class="content-badge">
        <span class="content-dot"></span>
        full content · ${(r.Tokens || 0).toLocaleString()} tokens
       </div>`
    : '';

  return `
    <div class="result">
      <div class="result-top">
        <span class="result-domain">${escHtml(r.Domain)}</span>
        <span class="result-engines">${escHtml(r.Engine)}</span>
        <div class="result-score">
          <div class="result-score-fill" style="width:${scorePct}%"></div>
        </div>
      </div>
      <div class="result-url">${escHtml(r.URL)}</div>
      <a class="result-title" href="${escHtml(r.URL)}" target="_blank" rel="noopener">
        ${escHtml(r.Title || r.URL)}
      </a>
      <div class="result-snippet">${escHtml(r.Snippet)}</div>
      ${contentBadge}
    </div>
  `;
}

function renderError(message) {
  container.innerHTML = `<div class="state-message error">${escHtml(message)}</div>`;
}

function escHtml(str) {
  if (!str) return "";
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
