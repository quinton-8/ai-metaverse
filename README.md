# 🌐 AI Metaverse

A powerful, multi-model AI interface built with a **Golang** backend and a **Next.js** frontend. This application allows you to seamlessly swap between different AI agents (like Gemini and GPT-4) from a single, centralized prompt bar.

---

## 📂 Project Structure

```text
ai-multiverse/
├── frontend/          # Next.js UI & Tailwind CSS (Runs on Port 3000)
│   ├── app/
│   │   └── page.tsx   # Main Chat Interface
│   └── package.json
│
└── backend/           # Golang AI Router (Runs on Port 8080)
    ├── .env           # API Keys (OpenAI, Gemini)
    ├── main.go        # Go Server & Model Logic
    └── go.mod         # Dependencies