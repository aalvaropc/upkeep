# Evaluation: notes-api

Mode `replay`, 1 run(s), version `v0.1.1-0.20261029080419-23f79ddf03c3`, 2026-10-29 08:04 UTC.

**9 of 9 cases ended as expected.** The 9 pull requests cost $0.0191 per run; the same tokens at the largest model's price would have cost $0.0502 (2.6 times as much). That is a price comparison, not a claim that the largest model would have fixed them.

| # | Pull request | Passed | Attempts | Model calls | Cost | At the largest price | Seconds |
|---|---|---|---|---|---|---|---|
| 1 | Search notes by title or body | 1/1 | 0.0 | 0.0 | $0.0000 | $0.0000 | 3.1 |
| 2 | Allow titles of up to 140 characters | 1/1 | 0.0 | 1.0 | $0.0005 | $0.0005 | 4.6 |
| 3 | Count how often each tag is used | 1/1 | 0.0 | 1.0 | $0.0016 | $0.0016 | 5.5 |
| 4 | Export all of a caller's notes as a download | 1/1 | 1.0 | 14.0 | $0.0041 | $0.0127 | 7.4 |
| 5 | Update github.com/golang-jwt/jwt/v4 to v5.3.1 | 1/1 | 1.0 | 7.0 | $0.0064 | $0.0213 | 4.4 |
| 6 | Bump github.com/rs/cors from 1.11.0 to 1.11.1 | 1/1 | 0.0 | 0.0 | $0.0000 | $0.0000 | 2.8 |
| 7 | Bump github.com/jonboulle/clockwork from 0.4.0 to 0.5.0 | 1/1 | 1.0 | 5.0 | $0.0031 | $0.0107 | 3.3 |
| 8 | Summarize how many notes and tags an owner has | 1/1 | 0.0 | 0.0 | $0.0000 | $0.0000 | 19.6 |
| 9 | Accept the token from a cookie and report its state | 1/1 | 0.0 | 1.0 | $0.0034 | $0.0034 | 0.0 |
