-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
ALTER TABLE products ADD FULLTEXT INDEX idx_fulltext_name_description (name, description);