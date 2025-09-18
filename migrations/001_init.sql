-- Create posts table
CREATE TABLE IF NOT EXISTS posts (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create GIN index for optimized tag searches
CREATE INDEX IF NOT EXISTS idx_posts_tags ON posts USING GIN (tags);

-- Create activity_logs table for transaction demonstration
CREATE TABLE IF NOT EXISTS activity_logs (
    id SERIAL PRIMARY KEY,
    action VARCHAR(50) NOT NULL,
    post_id INTEGER REFERENCES posts(id) ON DELETE CASCADE,
    logged_at TIMESTAMP DEFAULT NOW()
);

-- Create index for activity logs
CREATE INDEX IF NOT EXISTS idx_activity_logs_post_id ON activity_logs(post_id);
CREATE INDEX IF NOT EXISTS idx_activity_logs_logged_at ON activity_logs(logged_at);

-- Insert some sample data (optional)
INSERT INTO posts (title, content, tags) VALUES
    ('Getting Started with Go', 'Go is a statically typed, compiled programming language designed at Google.', ARRAY['golang', 'programming', 'backend']),
    ('Understanding Redis', 'Redis is an in-memory data structure store, used as a database, cache, and message broker.', ARRAY['redis', 'cache', 'database']),
    ('Elasticsearch Guide', 'Elasticsearch is a distributed, RESTful search and analytics engine.', ARRAY['elasticsearch', 'search', 'database']),
    ('PostgreSQL Best Practices', 'PostgreSQL is a powerful, open source object-relational database system.', ARRAY['postgresql', 'database', 'sql']),
    ('Docker for Development', 'Docker is a platform for developing, shipping, and running applications in containers.', ARRAY['docker', 'devops', 'containers'])
ON CONFLICT DO NOTHING;
