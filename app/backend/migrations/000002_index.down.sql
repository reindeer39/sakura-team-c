DROP INDEX idx_likes_post_created ON likes;
DROP INDEX idx_likes_created_post ON likes;
DROP INDEX idx_follows_followee_follower ON follows;
DROP INDEX idx_reposts_post_user ON reposts;

DROP INDEX idx_posts_parent_created_id ON posts;
DROP INDEX idx_posts_user_parent_created_id ON posts;

DROP INDEX idx_notifications_user_unread ON notifications;
DROP INDEX idx_notifications_user_created ON notifications;
DROP INDEX idx_footprints_user_visitor_created ON footprints;
