# Live evaluation: notes-api

Five runs of the whole scenario against the real Nemotron models on Nebius Token Factory (nothing replayed), on 2026-10-25. Per run, cost between $0.0198 and $0.0372, between 29 and 54 model calls, and every pull request ended as the scenario expects in every run. The replayed recording is in [`latest.md`](latest.md).

Mode `live`, 5 run(s), version `v0.0.0-20261025143129-cc27256756f0+dirty`, 2026-10-25 14:40 UTC.

**45 of 45 cases ended as expected.** The 9 pull requests cost $0.0270 per run; the same tokens at the largest model's price would have cost $0.0665 (2.5 times as much). That is a price comparison, not a claim that the largest model would have fixed them.

| # | Pull request | Passed | Attempts | Model calls | Cost | At the largest price | Seconds |
|---|---|---|---|---|---|---|---|
| 1 | Search notes by title or body | 5/5 | 0.0 | 0.0 | $0.0000 | $0.0000 | 11.7 |
| 2 | Allow titles of up to 140 characters | 5/5 | 0.0 | 1.0 | $0.0005 | $0.0005 | 13.0 |
| 3 | Count how often each tag is used | 5/5 | 0.0 | 2.0 | $0.0036 | $0.0036 | 14.1 |
| 4 | Export all of a caller's notes as a download | 5/5 | 1.2 | 13.4 | $0.0033 | $0.0099 | 28.4 |
| 5 | Update github.com/golang-jwt/jwt/v4 to v5.3.1 | 5/5 | 1.2 | 15.6 | $0.0146 | $0.0431 | 61.3 |
| 6 | Bump github.com/rs/cors from 1.11.0 to 1.11.1 | 5/5 | 0.0 | 0.0 | $0.0000 | $0.0000 | 3.2 |
| 7 | Bump github.com/jonboulle/clockwork from 0.4.0 to 0.5.0 | 5/5 | 1.0 | 4.2 | $0.0019 | $0.0063 | 9.9 |
| 8 | Summarize how many notes and tags an owner has | 5/5 | 0.0 | 0.0 | $0.0000 | $0.0000 | 11.2 |
| 9 | Accept the token from a cookie and report its state | 5/5 | 0.0 | 1.0 | $0.0031 | $0.0031 | 2.7 |
