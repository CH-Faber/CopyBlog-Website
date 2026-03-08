import os
from pathlib import Path
p = Path('src/content/posts/关于胡展榕的高中生活.md')
content = p.read_text(encoding='utf-8')
content = content.replace('tags: []', 'tags: ["TEST_PRESERVE"]')
p.write_text(content, encoding='utf-8')
os.utime(p, (1770000000.0, 1770000000.0))
print("Done touching file. New mtime:", os.path.getmtime(p))
