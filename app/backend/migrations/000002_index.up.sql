CREATE INDEX idx_likes_post_created ON likes (post_id, created_at);
CREATE INDEX idx_likes_created_post ON likes (created_at, post_id);
CREATE INDEX idx_follows_followee_follower ON follows (followee_id, follower_id);
CREATE INDEX idx_reposts_post_user ON reposts (post_id, user_id);

CREATE INDEX idx_posts_parent_created_id ON posts (parent_post_id, created_at, id);
CREATE INDEX idx_posts_user_parent_created_id ON posts (user_id, parent_post_id, created_at, id);

CREATE INDEX idx_notifications_user_unread ON notifications (user_id, is_read);
CREATE INDEX idx_notifications_user_created ON notifications (user_id, created_at, id);
CREATE INDEX idx_footprints_user_visitor_created ON footprints (user_id, visitor_id, created_at);
