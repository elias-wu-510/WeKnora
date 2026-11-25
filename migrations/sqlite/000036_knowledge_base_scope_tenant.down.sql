-- Reverse of 000036: restore the historical 'public' value. The column default
-- was never changed on SQLite (see the up migration), so only data flips back.

UPDATE knowledge_bases SET scope = 'public' WHERE scope = 'tenant';
