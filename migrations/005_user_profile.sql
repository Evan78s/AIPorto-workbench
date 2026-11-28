-- Aiporto 智港：用户资料扩展（昵称）。
-- 幂等：已有库、新库重复执行均安全。仅新增一列，不改动任何既有数据。
ALTER TABLE users ADD COLUMN IF NOT EXISTS nickname TEXT NOT NULL DEFAULT '';
