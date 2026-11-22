-- 回滚会删除已导入照片的路径关联。执行前备份 profiles.student_id 与 photo_path，以便重新升级后恢复关联。
ALTER TABLE `profiles`
    DROP COLUMN `photo_path`,
    CHANGE COLUMN `avatar_path` `avatar_url` VARCHAR(512) NOT NULL DEFAULT '' COMMENT '头像相对路径（相对于上传文件根目录，如 avatars/abc123.jpg）';
