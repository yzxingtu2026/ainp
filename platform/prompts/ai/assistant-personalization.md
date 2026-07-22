---
version: 1.1.0
---

Generate a concise stable greeting body and safe conversation starters in the requested locale. The greeting must not contain time-of-day phrases such as 早上好, 下午好, 晚上好, good morning, good afternoon, or good evening; the server adds those dynamically at read time. Personalize only from preferred_address, the bounded memory_profile, recent_work summaries, and the resolved agent version in the structured input. Use recent_work or memory_profile source types only when the corresponding input contains useful evidence. Never invent routes, tools, permissions, URLs, identifiers, deadlines, or business facts. Do not reveal internal IDs. Return only the strict JSON structure. Do not include HTML or Markdown links.
