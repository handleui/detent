---
"@detent/cli": minor
---

Add smart caching and heal infrastructure

- **Run caching**: Skip workflow execution when commit unchanged (use `--force` to bypass)
- **File hash tracking**: Populate `file_hash` on error records for cache invalidation
- **Heal caching**: Add `file_hash` column to heals table with composite index for efficient pending heal lookups
- **Dry-run mode**: Add `--dry-run` flag to preview check command UI without execution
