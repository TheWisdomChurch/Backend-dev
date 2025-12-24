-- scripts/init.sql
-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Testimonials table WITHOUT role and ratings
CREATE TABLE IF NOT EXISTS testimonials (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    full_name VARCHAR(200) GENERATED ALWAYS AS (first_name || ' ' || last_name) STORED,
    image_url VARCHAR(500),
    testimony TEXT NOT NULL,
    is_anonymous BOOLEAN DEFAULT FALSE,
    is_approved BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE -- ADD THIS LINE
);

-- Indexes
CREATE INDEX idx_testimonials_approved ON testimonials(is_approved);
CREATE INDEX idx_testimonials_created_at ON testimonials(created_at DESC);
CREATE INDEX idx_testimonials_deleted_at ON testimonials(deleted_at); -- ADD THIS LINE

-- Updated_at trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_testimonials_updated_at 
    BEFORE UPDATE ON testimonials
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insert sample testimonials WITHOUT role
INSERT INTO testimonials (first_name, last_name, testimony, is_approved) VALUES
    ('Michael', 'Johnson', 'I was lost in addiction for 15 years. Through the prayer ministry of this church and God''s grace, I''ve been sober for 3 years now. The support I received here changed my life completely.', true),
    ('Sarah', 'Williams', 'My family was going through a difficult financial season. Through the church''s benevolence ministry and the prayers of the saints, God miraculously provided for all our needs. To God be the glory!', true),
    ('Robert', 'Chen', 'After losing my job, I fell into depression. The counseling ministry and Bible study groups helped me find hope in God''s promises. Today, I have a better job and a stronger faith.', true),
    ('Grace', 'Okon', 'God healed me from a terminal illness after the church prayed for me. The doctors called it a miracle. I''m here today as a living testimony of God''s healing power.', true)
ON CONFLICT DO NOTHING;