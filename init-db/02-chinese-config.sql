-- Configure Chinese text search and word segmentation

-- Create Chinese text search configuration
CREATE TEXT SEARCH CONFIGURATION chinese (COPY = simple);
CREATE TEXT SEARCH DICTIONARY chinese_stem (TEMPLATE = simple, STOPWORDS = chinese);
ALTER TEXT SEARCH CONFIGURATION chinese ALTER MAPPING FOR word, asciiword WITH chinese_stem;

-- Create function to tokenize Chinese text (basic implementation)
CREATE OR REPLACE FUNCTION chinese_tokenize(input_text text)
RETURNS text[] AS $$
DECLARE
    tokens text[];
    i int;
    char_code int;
    current_token text := '';
    is_chinese boolean := false;
    prev_is_chinese boolean := false;
BEGIN
    tokens := ARRAY[]::text[];
    
    FOR i IN 1..length(input_text) LOOP
        char_code := ascii(substr(input_text, i, 1));
        is_chinese := (char_code > 127); -- Simple check for non-ASCII (Chinese) characters
        
        IF is_chinese != prev_is_chinese AND current_token != '' THEN
            -- Token boundary detected
            tokens := array_append(tokens, trim(current_token));
            current_token := '';
        END IF;
        
        current_token := current_token || substr(input_text, i, 1);
        prev_is_chinese := is_chinese;
    END LOOP;
    
    -- Add the last token
    IF current_token != '' THEN
        tokens := array_append(tokens, trim(current_token));
    END IF;
    
    -- Filter out empty strings and single characters
    SELECT array_agg(token)
    INTO tokens
    FROM unnest(tokens) AS token
    WHERE length(token) > 1 AND token !~ '^[[:space:]]*$';
    
    RETURN COALESCE(tokens, ARRAY[]::text[]);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Create function for Chinese text similarity search
CREATE OR REPLACE FUNCTION chinese_text_search(
    search_query text,
    target_table text DEFAULT 'documents',
    target_column text DEFAULT 'content',
    limit_count int DEFAULT 10
)
RETURNS TABLE (
    id UUID,
    title VARCHAR,
    content TEXT,
    rank FLOAT
) AS $$
DECLARE
    sql_query text;
BEGIN
    sql_query := format('
        SELECT 
            d.id,
            d.title,
            d.content,
            ts_rank_cd(to_tsvector(''chinese'', d.%I), plainto_tsquery(''chinese'', %L)) as rank
        FROM %I d
        WHERE to_tsvector(''chinese'', d.%I) @@ plainto_tsquery(''chinese'', %L)
        ORDER BY rank DESC
        LIMIT %s',
        target_column, search_query, target_table, target_column, search_query, limit_count
    );
    
    RETURN QUERY EXECUTE sql_query;
END;
$$ LANGUAGE plpgsql;

-- Create indexes for Chinese full-text search
CREATE INDEX IF NOT EXISTS idx_documents_chinese_fts ON documents 
    USING gin(to_tsvector('chinese', title || ' ' || content));

CREATE INDEX IF NOT EXISTS idx_text_chunks_chinese_fts ON text_chunks 
    USING gin(to_tsvector('chinese', content));