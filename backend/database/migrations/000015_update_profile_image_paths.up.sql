ALTER TABLE `profiles`
    CHANGE COLUMN `avatar_url` `avatar_path` VARCHAR(512) NOT NULL DEFAULT '' COMMENT '头像相对路径（相对于上传文件根目录，如 avatars/abc123.jpg）',
    ADD COLUMN `photo_path` VARCHAR(512) NULL DEFAULT NULL COMMENT '本人照片相对路径（相对于上传文件根目录，如 photo/abc123.jpg）' AFTER `avatar_path`;
