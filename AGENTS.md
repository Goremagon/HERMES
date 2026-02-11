# AGENTS.md
**Role:** GPT-5.3 Kodex (Execution Agent)
**Directives:**

1.  **Coding Style (Go):**
    * Prefer Standard Library where possible.
    * Handle errors explicitly (`if err != nil`).
    * Use Context for cancellation/timeouts.
    * Folder structure: `/cmd`, `/internal`, `/web` (frontend).

2.  **Coding Style (Vue/JS):**
    * Use Composition API (`<script setup>`).
    * TypeScript is PREFERRED but standard JS is allowed if simpler.
    * No complex build steps (Standard Vite).

3.  **Safety:**
    * NEVER commit secrets or hardcoded passwords.
    * Input validation on ALL API endpoints.
    * Sanitize messages to prevent XSS (escaped HTML).

4.  **Workflow:**
    * Always check `PROJECT_BIBLE.md` constraints before implementation.
    * If a requirement contradicts the Bible, HALT and ask Director.
