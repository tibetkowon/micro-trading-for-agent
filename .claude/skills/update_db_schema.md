# Skill: Update Database Schema Documentation

## Description
This skill ensures that the database schema documentation is always up-to-date. Since the project uses SQLite, maintaining an accurate Markdown representation of the schema is crucial to save tokens and avoid scanning raw `.db` or `.sql` files repeatedly.

## Trigger
Execute this skill whenever you create a new database table, alter an existing table's schema, modify database models in Go, or when the user explicitly requests a database schema update.

## Instructions
1. **Analyze Current Schema:** Review the latest SQLite migration files, Go models/structs, or raw SQL definitions in the backend codebase.
2. **Format as Markdown:** Structure the schema information into clear Markdown tables.
3. **Required Information:** For each table, you MUST provide:
   - Table Name and its business purpose.
   - Column Name
   - Data Type (SQLite specific)
   - Constraints (PK, FK, UNIQUE, NOT NULL, DEFAULT)
   - Description (Detailed explanation of what the column stores)
4. **Target File:** Update `docs/db_schema.md`. Replace the old schema representation with the newly analyzed one.
5. **Output Constraint:** Do not output the entire generated markdown table in the chat. Quietly update the file and simply notify the user: "The database schema documentation (`docs/db_schema.md`) has been updated successfully."
