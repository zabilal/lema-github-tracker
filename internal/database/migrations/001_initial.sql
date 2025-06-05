CREATE TABLE IF NOT EXISTS repositories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    url VARCHAR(500) NOT NULL,
    language VARCHAR(100),
    forks_count INTEGER DEFAULT 0,
    stars_count INTEGER DEFAULT 0,
    open_issues_count INTEGER DEFAULT 0,
    watchers_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_synced_at TIMESTAMP WITH TIME ZONE,
    last_commit_sha VARCHAR(40),
    sync_since TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at_db TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at_db TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS commits (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    sha VARCHAR(40) NOT NULL,
    message TEXT NOT NULL,
    author_name VARCHAR(255) NOT NULL,
    author_email VARCHAR(255) NOT NULL,
    commit_date TIMESTAMP WITH TIME ZONE NOT NULL,
    url VARCHAR(500) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(repository_id, sha)
);

CREATE INDEX IF NOT EXISTS idx_commits_repository_id ON commits(repository_id);
CREATE INDEX IF NOT EXISTS idx_commits_sha ON commits(sha);
CREATE INDEX IF NOT EXISTS idx_commits_author_name ON commits(author_name);
CREATE INDEX IF NOT EXISTS idx_commits_commit_date ON commits(commit_date);
CREATE INDEX IF NOT EXISTS idx_repositories_full_name ON repositories(full_name);