import fitz
doc = fitz.open('backend/tmp/statement_1760450625_OpTransactionHistory14-10-2025.pdf-12-05-23.pdf')
text = ''
for page in doc:
    text += page.get_text()
lines = text.split('\n')

# Look for column headers around line 30-40
print('Lines around transaction 1:')
for i in range(25, 45):
    if i < len(lines):
        print(f'Line {i}: "{lines[i]}"')
