import os
import boto3
import requests
import json
import re
from pathlib import Path
from botocore.client import Config
from datetime import datetime
from dotenv import load_dotenv

# 加载项目根目录的 .env 文件
load_dotenv(Path(__file__).parent.parent / ".env")

# --- 配置信息 ---
# 必须将这些信息放入根目录的 .env 文件中
ENDPOINT = os.getenv("S3_ENDPOINT", "")
REGION = os.getenv("S3_REGION", "")
ACCESS_KEY = os.getenv("S3_ACCESS_KEY", "")
SECRET_KEY = os.getenv("S3_SECRET_KEY", "")
BUCKET_NAME = os.getenv("S3_BUCKET_NAME", "")
PREFIX = os.getenv("S3_PREFIX", "website/")  # Obsidian 库中存放文章的文件夹路径

# --- AI API 配置 (用于生成 Frontmatter 头信息) ---
AI_API_KEY = os.getenv("AI_API_KEY", "")          # 填入你的API KEY
AI_BASE_URL = os.getenv("AI_BASE_URL", "https://api.openai.com/v1") # API Endpoint (比如 deepseek 等第三方兼容接口)
AI_MODEL = os.getenv("AI_MODEL", "gpt-4o-mini")   # 模型名称

# 博客本地存放文章的路径
LOCAL_POSTS_DIR = Path(__file__).parent.parent / "src" / "content" / "posts"

def get_s3_client():
    return boto3.client(
        's3',
        endpoint_url=ENDPOINT,
        aws_access_key_id=ACCESS_KEY,
        aws_secret_access_key=SECRET_KEY,
        region_name=REGION,
        config=Config(s3={'addressing_style': 'virtual'})
    )

def generate_frontmatter(filename, content):
    """调用第三方 AI 接口为没有头部信息的文章生成头信息"""
    if not AI_API_KEY:
        print(f"[{filename}] ⚠️ 未配置 AI_API_KEY，跳过 AI 生成，使用默认头信息。")
        return {
            "title": filename.replace(".md", ""),
            "description": "由 Obsidian 同步的文章",
            "category": "未分类",
            "tags": []
        }

    print(f"[{filename}] 🤖 正在使用 AI 解析并生成 Header 信息...")
    prompt = f"""请分析以下文章内容，并提取或生成适用作为博客系统头信息（Frontmatter）的 JSON 格式数据。
要求严格且只返回包含以下字段的 JSON 对象，不要含有任何额外文本或 Markdown 标记结构（例如不要用 ```json 包裹）：
{{
  "title": "文章标题（尽量使用原标题或精确总结）",
  "description": "1-2句话的文章内容摘要",
  "category": "文章分类（如: 技术, 随笔, 思考, 读书等，返回单个字符串）",
  "tags": ["标签1", "标签2", "标签3"] // 最多3-5个相关标签
}}

文章内容（只分析前2000字）：
{content[:2000]}
"""
    headers = {
        "Authorization": f"Bearer {AI_API_KEY}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": AI_MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.5,
        "max_tokens": 500
    }
    
    try:
        response = requests.post(f"{AI_BASE_URL}/chat/completions", headers=headers, json=payload, timeout=30)
        response.raise_for_status()
        result = response.json()
        ai_reply = result["choices"][0]["message"]["content"].strip()
        
        # 尝试提取或解析 JSON
        clean_reply = ai_reply.replace("```json", "").replace("```", "").strip()
        start = clean_reply.find('{')
        end = clean_reply.rfind('}')
        if start != -1 and end != -1:
            clean_reply = clean_reply[start:end+1]
        data = json.loads(clean_reply)
            
        return data
    except Exception as e:
        print(f"[{filename}] ❌ AI 生成失败: {e}")
        return {
            "title": filename.replace(".md", ""),
            "description": "AI 生成失败，由 Obsidian 同步",
            "category": "未分类",
            "tags": []
        }

def extract_frontmatter(content):
    """提取 markdown 内容中的 frontmatter 和剩余 body"""
    if str(content).lstrip().startswith("---"):
        parts = content.split("---", 2)
        if len(parts) >= 3:
            return "---" + parts[1] + "---", parts[2].lstrip()
    return None, content

def build_fm_string(ai_data, filename, s3_mtime):
    """将 AI 生成的数据转换为 Frontmatter 字符串"""
    published_date = datetime.fromtimestamp(s3_mtime).strftime("%Y-%m-%dT%H:%M:%S.000Z")
    title = str(ai_data.get("title", filename.replace(".md", ""))).replace('"', '\\"')
    description = str(ai_data.get("description", "")).replace('"', '\\"')
    category = str(ai_data.get("category", "未分类"))
    tags = ai_data.get("tags", [])
    if not isinstance(tags, list):
        tags = []
    
    frontmatter_lines = [
        "---",
        f'title: "{title}"',
        f'published: {published_date}',
        f'description: "{description}"',
        f'category: "{category}"',
        f'tags: {json.dumps(tags, ensure_ascii=False)}',
        "---"
    ]
    return "\n".join(frontmatter_lines)

def sync_articles():
    s3 = get_s3_client()
    LOCAL_POSTS_DIR.mkdir(parents=True, exist_ok=True)
    
    print(f"开始从 {BUCKET_NAME}/{PREFIX} 同步文章...")
    
    paginator = s3.get_paginator('list_objects_v2')
    pages = paginator.paginate(Bucket=BUCKET_NAME, Prefix=PREFIX)
    
    count = 0
    for page in pages:
        if 'Contents' not in page:
            continue
            
        for obj in page['Contents']:
            key = obj['Key']
            if not key.endswith('.md'):
                continue
                
            filename = os.path.basename(key)
            local_path = LOCAL_POSTS_DIR / filename
            s3_mtime = obj['LastModified'].timestamp()
            
            if local_path.exists():
                local_mtime = os.path.getmtime(local_path)
                if s3_mtime <= local_mtime:
                    continue  # 已是最新，跳过
                
                # 本地存在但云端更新：保留可能原本有的本地 frontmatter
                print(f"[{filename}] ♻️ 正在同步更新...")
                try:
                    with open(local_path, 'r', encoding='utf-8') as f:
                        local_content = f.read()
                    existing_fm, _ = extract_frontmatter(local_content)
                    
                    # 下载最新内容到临时文件
                    temp_path = str(local_path) + ".tmp"
                    s3.download_file(BUCKET_NAME, key, temp_path)
                    
                    with open(temp_path, 'r', encoding='utf-8') as f:
                        new_content = f.read()
                    
                    new_fm, new_body = extract_frontmatter(new_content)
                    
                    # 决策合并方案
                    if new_fm:
                        final_content = new_fm + "\n\n" + new_body
                    elif existing_fm:
                        final_content = existing_fm + "\n\n" + new_body
                    else:
                        ai_fm_data = generate_frontmatter(filename, new_body)
                        final_fm_str = build_fm_string(ai_fm_data, filename, s3_mtime)
                        final_content = final_fm_str + "\n\n" + new_body
                        
                    # 覆写本地文件
                    with open(local_path, 'w', encoding='utf-8') as f:
                        f.write(final_content)
                        
                    os.remove(temp_path)
                    count += 1
                except Exception as e:
                    print(f"[{filename}] ❌ 更新文件时发生错误: {e}")
                
            else:
                # 本地不存在，全新下载
                print(f"[{filename}] 🆕 正在同步新文章...")
                s3.download_file(BUCKET_NAME, key, str(local_path))
                
                try:
                    with open(local_path, 'r', encoding='utf-8') as f:
                        content = f.read()
                    
                    new_fm, body = extract_frontmatter(content)
                    if not new_fm:
                        ai_fm_data = generate_frontmatter(filename, body)
                        final_fm_str = build_fm_string(ai_fm_data, filename, s3_mtime)
                        final_content = final_fm_str + "\n\n" + body
                        with open(local_path, 'w', encoding='utf-8') as f:
                            f.write(final_content)
                    
                    count += 1
                except Exception as e:
                    print(f"[{filename}] ❌ 处理新文件时发生错误: {e}")

    print(f"同步完成！共计下载/更新了 {count} 篇文章。")

if __name__ == "__main__":
    try:
        sync_articles()
    except Exception as e:
        print(f"同步失败: {str(e)}")
        print("\n提示: 如果提示找不到 boto3 或者 requests，请运行: pip install boto3 requests")
