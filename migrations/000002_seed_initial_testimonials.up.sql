-- Insert sample testimonials WITHOUT role
INSERT INTO testimonials (first_name, last_name, testimony, is_approved) VALUES
    ('Michael', 'Johnson', 'I was lost in addiction for 15 years. Through the prayer ministry of this church and God''s grace, I''ve been sober for 3 years now. The support I received here changed my life completely.', true),
    ('Sarah', 'Williams', 'My family was going through a difficult financial season. Through the church''s benevolence ministry and the prayers of the saints, God miraculously provided for all our needs. To God be the glory!', true),
    ('Robert', 'Chen', 'After losing my job, I fell into depression. The counseling ministry and Bible study groups helped me find hope in God''s promises. Today, I have a better job and a stronger faith.', true),
    ('Grace', 'Okon', 'God healed me from a terminal illness after the church prayed for me. The doctors called it a miracle. I''m here today as a living testimony of God''s healing power.', true)
ON CONFLICT DO NOTHING;