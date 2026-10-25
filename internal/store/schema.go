package store

var migrations = [][]string{
	{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			mode TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		) STRICT`,
		`CREATE TABLE runs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL,
			finished_at INTEGER
		) STRICT`,
		`CREATE INDEX runs_session ON runs(session_id)`,
		`CREATE TABLE prs (
			id INTEGER PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			number INTEGER NOT NULL,
			title TEXT NOT NULL,
			author_login TEXT NOT NULL,
			author_kind TEXT NOT NULL,
			kind TEXT NOT NULL,
			blocker TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			outcome TEXT NOT NULL DEFAULT '',
			base TEXT NOT NULL,
			branch TEXT NOT NULL,
			head_sha TEXT NOT NULL DEFAULT '',
			UNIQUE (run_id, number)
		) STRICT`,
		`CREATE TABLE steps (
			id INTEGER PRIMARY KEY,
			pr_id INTEGER NOT NULL REFERENCES prs(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			finished_at INTEGER,
			detail TEXT NOT NULL DEFAULT '',
			UNIQUE (pr_id, seq)
		) STRICT`,
		`CREATE TABLE model_calls (
			id INTEGER PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			pr_id INTEGER REFERENCES prs(id) ON DELETE CASCADE,
			step_id INTEGER REFERENCES steps(id) ON DELETE CASCADE,
			role TEXT NOT NULL,
			model TEXT NOT NULL,
			source TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			latency_ms INTEGER NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			reasoning_tokens INTEGER NOT NULL,
			cost_micros INTEGER,
			baseline_micros INTEGER
		) STRICT`,
		`CREATE INDEX model_calls_run ON model_calls(run_id)`,
		`CREATE TABLE actions (
			id INTEGER PRIMARY KEY,
			pr_id INTEGER NOT NULL REFERENCES prs(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		) STRICT`,
		`CREATE TABLE events (
			id INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			kind TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			UNIQUE (session_id, seq)
		) STRICT`,
	},
}
