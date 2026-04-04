#!/usr/bin/env python3
"""
Web Search Tool using Baidu Search

This tool:
1. Searches the web using Baidu web search
2. Ranks search results using Lexrank algorithm based on title relevance
3. Crawls the actual content from top-ranked search result pages using Selenium
4. Extracts relevant snippets using TextRank algorithm
5. Returns the most relevant content snippets matching the search query

Features:
- Full JavaScript support via Selenium WebDriver
- Automatic ChromeDriver management
- Headless Chrome for efficient crawling
- Handles dynamic content loading and SPAs
- Reuses single Chrome instance for all operations
"""

import argparse
import shutil
import re
import sys
import time
import random
from html import unescape
from collections import defaultdict
import math
import os
from urllib.parse import quote
from datetime import datetime
import json

# Baidu search
from baidusearch.baidusearch import search

# Timestamp file for rate limiting
TIMESTAMP_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".baidu_search_last_call")
SEARCH_INTERVAL = 15  # Minimum interval between searches in seconds

# Selenium imports
from selenium import webdriver
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import (
    TimeoutException,
    WebDriverException,
    NoSuchElementException,
    StaleElementReferenceException
)
from selenium.webdriver.common.action_chains import ActionChains


# ============================================================================
# Global Chrome Driver Manager - Reuse single instance
# ============================================================================

_global_driver = None
_driver_config = {
    "user_agents": [
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1.1 Safari/605.1.15",
    ],
    "window_sizes": [
        # (1920, 1080),
        (1366, 768),
        (1536, 864),
        (1440, 900),
        (1280, 720),
    ]
}


def _random_user_agent() -> str:
    """Get a random user agent string."""
    return random.choice(_driver_config["user_agents"])


def _random_window_size() -> tuple:
    """Get a random window size."""
    return random.choice(_driver_config["window_sizes"])


def _add_stealth_options(driver) -> None:
    """Add stealth options to bypass detection."""
    try:
        driver.execute_cdp_cmd('Page.addScriptToEvaluateOnNewDocument', {
            'source': '''
                Object.defineProperty(navigator, 'webdriver', {
                    get: () => undefined
                });
                Object.defineProperty(navigator, 'plugins', {
                    get: () => [1, 2, 3, 4, 5]
                });
                Object.defineProperty(navigator, 'languages', {
                    get: () => ['zh-CN', 'zh', 'en-US', 'en']
                });
                Object.defineProperty(navigator, 'hardwareConcurrency', {
                    get: () => 8
                });
                Object.defineProperty(navigator, 'deviceMemory', {
                    get: () => 8
                });
                window.chrome = {
                    runtime: {},
                    loadTimes: function() {},
                    csi: function() {},
                    app: {}
                };
                const originalQuery = window.navigator.permissions.query;
                window.navigator.permissions.query = (parameters) => (
                    parameters.name === 'notifications' ?
                        Promise.resolve({ state: Notification.permission }) :
                        originalQuery(parameters)
                );
                delete navigator.__proto__.webdriver;
            '''
        })
    except Exception as e:
        print(f"  Warning: Could not add all stealth options: {e}", file=sys.stderr)


def get_chrome_driver(headless: bool = True, use_profile: bool = True) -> webdriver.Chrome:
    """
    Get or create a shared Chrome driver instance.

    Args:
        headless: Whether to run in headless mode (default: True for background execution)
        use_profile: Whether to use a user data directory

    Returns:
        Shared Chrome WebDriver instance
    """
    global _global_driver

    if _global_driver is not None:
        try:
            _global_driver.execute_script("return navigator.userAgent")
            return _global_driver
        except Exception:
            _global_driver = None
            try:
                _global_driver.quit()
            except:
                pass

    script_dir = os.path.dirname(os.path.abspath(__file__))
    chrome_options = Options()

    # Headless mode
    if headless:
        chrome_options.add_argument("--headless=new")

    user_agent = _random_user_agent()
    chrome_options.add_argument(f"--user-agent={user_agent}")

    width, height = _random_window_size()
    chrome_options.add_argument(f"--window-size={width},{height}")
    chrome_options.add_argument("--start-minimized")

    chrome_options.add_argument("--no-sandbox")
    chrome_options.add_argument("--disable-dev-shm-usage")
    chrome_options.add_argument("--disable-gpu")
    chrome_options.add_argument("--disable-blink-features=AutomationControlled")
    chrome_options.add_experimental_option("excludeSwitches", ["enable-automation", "enable-logging"])
    chrome_options.add_experimental_option('useAutomationExtension', False)
    chrome_options.add_argument("--disable-extensions")
    chrome_options.add_argument("--disable-background-networking")
    chrome_options.add_argument("--disable-default-apps")
    chrome_options.add_argument("--disable-sync")
    chrome_options.add_argument("--disable-translate")
    chrome_options.add_argument("--metrics-recording-only")
    chrome_options.add_argument("--no-first-run")
    chrome_options.add_argument("--safebrowsing-disable-auto-update")
    chrome_options.add_argument("--log-level=3")
    chrome_options.add_argument("--silent")

    if use_profile:
        user_data_dir = os.path.join(script_dir, "chrome_user_data")
        try:
            if os.path.exists(user_data_dir):
                shutil.rmtree(user_data_dir)
        except Exception as e:
            pass
        os.makedirs(user_data_dir, exist_ok=True)
        chrome_options.add_argument(f"--user-data-dir={user_data_dir}")

    prefs = {
        "profile.managed_default_content_settings.images": 2,
        "profile.default_content_setting_values.stylesheets": 2,
        "profile.default_content_setting_values.cookies": 1,
        "profile.default_content_setting_values.notifications": 2,
        "profile.default_content_setting_values.popups": 2,
        "profile.default_content_setting_values.geolocation": 2,
        "profile.default_content_setting_values.media_stream": 2,
        "safebrowsing.enabled": True,
        # 不恢复上次退出未关闭的标签页
        "exit_type": "Clean",
        "profile.default_content_setting_values.automatic_downloads": 1,
        # 禁用会话恢复
        "session_restore_enabled": False,
    }
    chrome_options.add_experimental_option("prefs", prefs)

    chrome_options.binary_location = os.path.join(script_dir, "chrome-win64", "chrome-websearch.exe")
    chromedriver_path = os.path.join(script_dir, "chromedriver-win64", "chromedriver.exe")

    try:
        service = Service(chromedriver_path)
        _global_driver = webdriver.Chrome(service=service, options=chrome_options)
    except Exception as e:
        print(f"  Warning: Service initialization failed, using fallback: {e}", file=sys.stderr)
        _global_driver = webdriver.Chrome(executable_path=chromedriver_path, options=chrome_options)

    _add_stealth_options(_global_driver)

    try:
        _global_driver.execute_script("""
            Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
            Object.defineProperty(navigator, 'plugins', {get: () => [1, 2, 3, 4, 5]});
            Object.defineProperty(navigator, 'languages', {get: () => ['zh-CN', 'zh', 'en-US', 'en']});
        """)
    except Exception as e:
        print(f"  Warning: Could not execute stealth script: {e}", file=sys.stderr)

    # 关闭所有已打开的标签页，确保从一个干净的窗口开始
    try:
        _global_driver.switch_to.window(_global_driver.window_handles[0])
        # 关闭所有多余的标签页，只保留一个
        while len(_global_driver.window_handles) > 1:
            _global_driver.switch_to.window(_global_driver.window_handles[-1])
            _global_driver.close()
        # 确保当前标签页是干净的 about:blank 页面
        _global_driver.get("about:blank")
    except Exception as e:
        print(f"  Warning: Could not clean up tabs: {e}", file=sys.stderr)

    _global_driver.set_page_load_timeout(60)

    return _global_driver


def close_chrome_driver():
    """Close the shared Chrome driver instance and clean up all Chrome processes."""
    global _global_driver
    if _global_driver is not None:
        try:
            # 先关闭所有窗口
            for handle in _global_driver.window_handles:
                try:
                    _global_driver.switch_to.window(handle)
                    _global_driver.close()
                except:
                    pass
        except:
            pass
        
        try:
            # 尝试正常退出
            _global_driver.quit()
        except Exception:
            pass
        
        _global_driver = None
    
    # 在 Windows 上，Chrome 进程可能不会完全退出，需要强制清理
    if os.name == 'nt':
        try:
            import subprocess
            # 强制结束所有 chrome-websearch.exe 和 chromedriver.exe 进程
            subprocess.run(['taskkill', '/F', '/IM', 'chrome-websearch.exe'], 
                          capture_output=True, timeout=5)
            subprocess.run(['taskkill', '/F', '/IM', 'chromedriver.exe'], 
                          capture_output=True, timeout=5)
        except Exception:
            pass
    
    # 清理用户数据目录，避免锁定问题
    try:
        script_dir = os.path.dirname(os.path.abspath(__file__))
        user_data_dir = os.path.join(script_dir, "chrome_user_data")
        if os.path.exists(user_data_dir):
            # 延迟删除，给进程一点时间释放文件锁
            time.sleep(0.5)
            shutil.rmtree(user_data_dir, ignore_errors=True)
    except Exception:
        pass


# ============================================================================
# Rate Limiting Module
# ============================================================================

def ensure_search_interval() -> None:
    """
    Ensure minimum interval between Baidu search calls.
    Reads the last call timestamp from file and waits if necessary.
    """
    current_time = datetime.now().timestamp()
    
    if os.path.exists(TIMESTAMP_FILE):
        try:
            with open(TIMESTAMP_FILE, 'r') as f:
                last_call = float(f.read().strip())
            elapsed = current_time - last_call
            if elapsed < SEARCH_INTERVAL:
                wait_time = SEARCH_INTERVAL - elapsed
                print(f"  Rate limit: waiting {wait_time:.1f} seconds...", file=sys.stderr)
                time.sleep(wait_time)
        except (ValueError, IOError) as e:
            print(f"  Warning: Could not read timestamp file: {e}", file=sys.stderr)
    
    # Update timestamp file with current time
    try:
        with open(TIMESTAMP_FILE, 'w') as f:
            f.write(str(current_time))
    except IOError as e:
        print(f"  Warning: Could not write timestamp file: {e}", file=sys.stderr)


# ============================================================================
# Web Search Module - Baidu Search
# ============================================================================

def baidu_search(query: str, num_results: int = 5) -> dict:
    """
    Search using Baidu web search.

    Args:
        query: Search query
        num_results: Number of results to fetch
    """
    try:
        # Ensure minimum interval between searches
        ensure_search_interval()
        
        print(f"  Searching on Baidu: {query}", file=sys.stderr)
        
        # Use baidusearch library
        results = search(query, num_results=num_results)
        
        if not results:
            return {
                "success": True,
                "query": query,
                "results": [],
                "total_results": 0,
                "message": "No results found."
            }
        
        # Convert to expected format
        search_results = []
        for item in results[:num_results]:
            search_results.append({
                "title": item.get("title", ""),
                "url": item.get("url", ""),
                "snippet": item.get("abstract", "")
            })
        
        print(f"  Retrieved {len(search_results)} results from Baidu", file=sys.stderr)
        
        return {
            "success": True,
            "query": query,
            "results": search_results,
            "total_results": len(search_results)
        }
        
    except Exception as e:
        print(f"  Search error: {str(e)}", file=sys.stderr)
        return {"success": False, "error": f"Search error: {str(e)}"}


# ============================================================================
# Web Crawler Module - Selenium-based Page Content Extraction
# ============================================================================

def fetch_page_content(url: str, timeout: int = 30, max_retries: int = 2) -> tuple:
    """
    Fetch and extract main content from a web page using shared Chrome driver.

    Args:
        url: The URL to fetch
        timeout: Request timeout in seconds
        max_retries: Maximum retry attempts

    Returns:
        Tuple of (title, main_content) or (None, None) on error
    """
    driver = None

    for attempt in range(max_retries + 1):
        try:
            driver = get_chrome_driver(True)
            driver.set_page_load_timeout(timeout)

            print(f"  Loading: {url[:80]}...", file=sys.stderr)

            try:
                driver.execute_script(f"window.open('{url}', '_blank');")
                driver.switch_to.window(driver.window_handles[-1])
                WebDriverWait(driver, timeout).until(
                    lambda d: d.execute_script("return document.readyState") == "complete"
                )
            except TimeoutException:
                print(f"  Page load timeout, using partial content", file=sys.stderr)

            time.sleep(2)

            try:
                WebDriverWait(driver, 5).until(
                    EC.presence_of_element_located((By.TAG_NAME, "body"))
                )
            except TimeoutException:
                pass

            try:
                title = driver.title.strip() if driver.title else ""
            except Exception:
                title = ""

            content = extract_main_content_js(driver)

            if content and len(content) > 200:
                valid_chars = sum(1 for c in content[:500] if c.isprintable() or c in '\n\r\t')
                if valid_chars / min(500, len(content)) > 0.7:
                    return title, content

            print(f"  Content too short or invalid ({len(content) if content else 0} chars)", file=sys.stderr)

        except TimeoutException:
            print(f"  Timeout (attempt {attempt+1}/{max_retries+1})", file=sys.stderr)
        except WebDriverException as e:
            print(f"  WebDriver Error: {str(e)[:80]} (attempt {attempt+1}/{max_retries+1})", file=sys.stderr)
        except Exception as e:
            print(f"  Error: {type(e).__name__}: {str(e)[:80]} (attempt {attempt+1}/{max_retries+1})", file=sys.stderr)

        if attempt < max_retries:
            time.sleep(2 ** attempt + random.uniform(0, 2))

    return None, None


def extract_main_content_js(driver) -> str:
    """
    Extract main content from web page using JavaScript.
    """
    js_script = """
    (function() {
        function getVisibleText(el) {
            if (!el) return '';
            const style = window.getComputedStyle(el);
            if (style.display === 'none' || style.visibility === 'hidden') {
                return '';
            }
            return el.innerText || el.textContent || '';
        }

        const mainSelectors = [
            'article', 'main',
            '.post-content', '.article-content', '.entry-content',
            '.post', '.article', '#content', '.content',
            '[role="main"]', '.main-content', '#main-content',
            '.entry', '.post-body', '.article-body',
            '#article-content', '.text-content', '.article-main'
        ];

        let mainContent = null;

        for (const selector of mainSelectors) {
            const el = document.querySelector(selector);
            if (el) {
                const text = getVisibleText(el);
                if (text && text.trim().length > 200) {
                    mainContent = el;
                    break;
                }
            }
        }

        if (!mainContent) {
            const allElements = document.querySelectorAll('*');
            let maxTextLen = 0;
            let bestElement = null;

            for (const el of allElements) {
                const tagName = el.tagName.toLowerCase();
                if (['script', 'style', 'noscript', 'iframe', 'svg', 'canvas'].includes(tagName)) {
                    continue;
                }

                const role = el.getAttribute('role');
                if (role === 'navigation' || role === 'banner' || role === 'contentinfo') {
                    continue;
                }

                const text = getVisibleText(el);
                if (text && text.trim().length > maxTextLen) {
                    const childText = Array.from(el.children)
                        .map(c => getVisibleText(c))
                        .join('')
                        .trim().length;

                    if (text.trim().length > 500 && text.trim().length > childText * 0.7) {
                        maxTextLen = text.trim().length;
                        bestElement = el;
                    }
                }
            }

            if (bestElement && maxTextLen > 500) {
                mainContent = bestElement;
            }
        }

        if (!mainContent) {
            mainContent = document.body;
        }

        const textElements = mainContent.querySelectorAll('p, h1, h2, h3, h4, h5, h6, li, blockquote, pre, code, div[class*="content"], div[class*="text"]');
        let result = [];

        const directText = getVisibleText(mainContent);
        if (directText && directText.trim().length > 100) {
            result.push(directText);
        }

        textElements.forEach(el => {
            const text = getVisibleText(el);
            if (text && text.trim().length > 20) {
                const isDuplicate = result.some(r => r.includes(text.trim().substring(0, 50)));
                if (!isDuplicate) {
                    result.push(text.trim());
                }
            }
        });

        if (result.length === 0) {
            return getVisibleText(document.body);
        }

        return result.join('\\n\\n');
    })();
    """

    try:
        content = driver.execute_script(js_script)
        if content and isinstance(content, str):
            cleaned = clean_text(content)
            if cleaned and len(cleaned) > 200:
                return cleaned

        print(f"  JS extraction returned empty/short content, using fallback", file=sys.stderr)
        try:
            body_text = driver.find_element(By.TAG_NAME, "body").text
            if body_text and len(body_text) > 200:
                return clean_text(body_text)
        except Exception as fallback_error:
            print(f"  Fallback also failed: {fallback_error}", file=sys.stderr)

        return ""

    except Exception as e:
        print(f"  JS extraction error: {e}", file=sys.stderr)
        try:
            return driver.find_element(By.TAG_NAME, "body").text
        except:
            return ""


def clean_text(text: str) -> str:
    """Clean and normalize text."""
    text = unescape(text)
    text = re.sub(r'\n{3,}', '\n\n', text)
    text = re.sub(r' {2,}', ' ', text)
    return text.strip()


# ============================================================================
# Lexrank Algorithm - For Search Result Ranking
# ============================================================================

class Lexrank:
    """Lexrank implementation for ranking search results based on title similarity."""

    def __init__(self, damping: float = 0.85, iterations: int = 30, convergence: float = 0.0001):
        self.damping = damping
        self.iterations = iterations
        self.convergence = convergence

    def tokenize(self, text: str) -> list:
        """Tokenize text into words (supports Chinese and English)."""
        text = text.lower()
        stopwords = {
            'the', 'a', 'an', 'and', 'or', 'but', 'in', 'on', 'at', 'to', 'for',
            'of', 'with', 'by', 'from', 'is', 'are', 'was', 'were', 'be', 'been',
            'being', 'have', 'has', 'had', 'do', 'does', 'did', 'will', 'would',
            'could', 'should', 'may', 'might', 'must', 'shall', 'can', 'need',
            'this', 'that', 'these', 'those', 'it', 'its', 'as', 'if', 'when',
            'than', 'because', 'while', 'although', 'though', 'after', 'before',
            'until', 'unless', 'where', 'whether', 'which', 'who', 'whom', 'whose',
            'what', 'whatever', 'which', 'whichever', 'whoever', 'whomever',
            '的', '了', '在', '是', '就', '和', '与', '及', '或', '等', '着', '也',
            '都', '而', '及', '到', '得', '给', '关于', '作为', '对于', '如果',
            '虽然', '但是', '因为', '所以', '可以', '能够', '可能', '应该',
        }
        tokens = re.findall(r'[\u4e00-\u9fa5]+|[a-zA-Z]+', text)
        return [t for t in tokens if t.lower() not in stopwords and len(t) > 1]

    def compute_tf(self, documents: list) -> list:
        """Compute term frequency for each document."""
        tf_scores = []
        for doc in documents:
            tf = defaultdict(float)
            total_terms = len(doc)
            if total_terms == 0:
                tf_scores.append({})
                continue
            for term in doc:
                tf[term] += 1
            for term in tf:
                tf[term] /= total_terms
            tf_scores.append(tf)
        return tf_scores

    def compute_idf(self, documents: list) -> dict:
        """Compute inverse document frequency."""
        doc_count = len(documents)
        term_doc_count = defaultdict(int)

        for doc in documents:
            unique_terms = set(doc)
            for term in unique_terms:
                term_doc_count[term] += 1

        idf = {}
        for term, count in term_doc_count.items():
            idf[term] = math.log(doc_count / count) + 1

        return idf

    def compute_tf_idf(self, documents: list) -> list:
        """Compute TF-IDF vectors for documents."""
        tf_scores = self.compute_tf(documents)
        idf_scores = self.compute_idf(documents)

        tf_idf_scores = []
        for tf in tf_scores:
            tf_idf = {}
            for term, tf_val in tf.items():
                tf_idf[term] = tf_val * idf_scores.get(term, 1.0)
            tf_idf_scores.append(tf_idf)

        return tf_idf_scores

    def cosine_similarity(self, vec1: dict, vec2: dict) -> float:
        """Compute cosine similarity between two TF-IDF vectors."""
        common_terms = set(vec1.keys()) & set(vec2.keys())
        if not common_terms:
            return 0.0

        dot_product = sum(vec1[term] * vec2[term] for term in common_terms)
        norm1 = math.sqrt(sum(v ** 2 for v in vec1.values()))
        norm2 = math.sqrt(sum(v ** 2 for v in vec2.values()))

        if norm1 == 0 or norm2 == 0:
            return 0.0

        return dot_product / (norm1 * norm2)

    def rank(self, texts: list, query: str = None, top_n: int = None) -> list:
        """
        Rank texts using Lexrank algorithm.

        Args:
            texts: List of text strings to rank
            query: Optional query string to boost relevance
            top_n: Number of top results to return (None for all)

        Returns:
            List of (index, score) tuples sorted by score descending
        """
        if not texts:
            return []

        if len(texts) == 1:
            return [(0, 1.0)]

        documents = [self.tokenize(text) for text in texts]
        documents = [doc for doc in documents if doc]

        if len(documents) < 2:
            return [(0, 1.0)] if documents else []

        tf_idf_vectors = self.compute_tf_idf(documents)

        n = len(tf_idf_vectors)
        similarity_matrix = [[0.0] * n for _ in range(n)]

        for i in range(n):
            for j in range(i + 1, n):
                sim = self.cosine_similarity(tf_idf_vectors[i], tf_idf_vectors[j])
                similarity_matrix[i][j] = sim
                similarity_matrix[j][i] = sim

        transition_matrix = []
        for i in range(n):
            row_sum = sum(similarity_matrix[i])
            if row_sum > 0:
                transition_matrix.append([sim / row_sum for sim in similarity_matrix[i]])
            else:
                transition_matrix.append([1.0 / n] * n)

        scores = [1.0 / n] * n

        for _ in range(self.iterations):
            prev_scores = scores.copy()
            convergence_met = True

            for i in range(n):
                rank = (1 - self.damping) / n
                for j in range(n):
                    rank += self.damping * prev_scores[j] * transition_matrix[j][i]
                scores[i] = rank
                if abs(scores[i] - prev_scores[i]) > self.convergence:
                    convergence_met = False

            if convergence_met:
                break

        if query:
            query_tokens = set(self.tokenize(query))
            for i, doc in enumerate(documents):
                doc_tokens = set(doc)
                keyword_match = len(doc_tokens & query_tokens)
                query_in_text = 1.0 if query.lower() in texts[i].lower() else 0.0
                scores[i] *= (1 + 0.3 * keyword_match + 0.5 * query_in_text)

        indexed_scores = list(enumerate(scores))
        indexed_scores.sort(key=lambda x: x[1], reverse=True)

        if top_n is not None:
            return indexed_scores[:top_n]
        return indexed_scores


# ============================================================================
# TextRank Algorithm
# ============================================================================

class TextRank:
    """TextRank implementation for keyword extraction and sentence ranking."""

    def __init__(self, damping: float = 0.85, iterations: int = 30, convergence: float = 0.0001):
        self.damping = damping
        self.iterations = iterations
        self.convergence = convergence

    def tokenize(self, text: str) -> list:
        """Tokenize text into words (supports Chinese and English)."""
        text = text.lower()
        return re.findall(r'[\u4e00-\u9fa5]+|[a-zA-Z]+', text)

    def split_sentences(self, text: str) -> list:
        """Split text into sentences."""
        sentences = re.split(r'[.!?。！？\n]+', text)
        return [s.strip() for s in sentences if s.strip()]

    def split_paragraphs(self, text: str, min_length: int = 80) -> list:
        """Split text into paragraphs."""
        paragraphs = re.split(r'\n\s*\n', text)
        result = []
        for para in paragraphs:
            para = para.strip()
            if len(para) >= min_length:
                result.append(para)
            elif len(para) >= 20:
                lines = [l.strip() for l in para.split('\n') if l.strip()]
                for line in lines:
                    if len(line) >= 30:
                        result.append(line)
                    else:
                        sentences = self.split_sentences(line)
                        result.extend([s for s in sentences if len(s) >= 30])
        return result

    def build_graph(self, items: list, window_size: int = 4) -> dict:
        """Build a co-occurrence graph."""
        graph = defaultdict(lambda: defaultdict(float))
        for i in range(len(items)):
            word = items[i]
            for j in range(i + 1, min(i + window_size + 1, len(items))):
                neighbor = items[j]
                if word != neighbor:
                    graph[word][neighbor] += 1.0
                    graph[neighbor][word] += 1.0
        return graph

    def pagerank(self, graph: dict) -> dict:
        """Compute PageRank scores."""
        scores = {}
        nodes = list(graph.keys())
        if not nodes:
            return {}

        for node in nodes:
            scores[node] = 1.0

        for _ in range(self.iterations):
            prev_scores = scores.copy()
            convergence_met = True

            for node in nodes:
                total_weight = sum(graph[node].values())
                if total_weight == 0:
                    continue

                rank = (1 - self.damping)
                for neighbor, weight in graph[node].items():
                    neighbor_total = sum(graph[neighbor].values())
                    if neighbor_total > 0:
                        rank += self.damping * (weight / neighbor_total) * prev_scores.get(neighbor, 0)

                scores[node] = rank
                if abs(scores[node] - prev_scores[node]) > self.convergence:
                    convergence_met = False

            if convergence_met:
                break

        return scores

    def rank_sentences(self, text: str, query: str, top_n: int = 30) -> list:
        """Rank sentences/paragraphs by relevance to query."""
        paragraphs = self.split_paragraphs(text, min_length=500)
        if not paragraphs:
            paragraphs = self.split_sentences(text)

        if not paragraphs:
            return []

        paragraphs = [p for p in paragraphs if len(p) >= 20]
        if not paragraphs:
            return []

        query_keywords = set(self.tokenize(query))
        paragraph_vectors = [(para, set(self.tokenize(para))) for para in paragraphs]

        if len(paragraph_vectors) == 1:
            para, words = paragraph_vectors[0]
            keyword_match = len(words & query_keywords)
            query_in_para = 1.0 if query.lower() in para.lower() else 0.0
            score = 1.0 * (1 + 0.3 * keyword_match + 0.5 * query_in_para)
            return [(para, score)] if score > 1.0 else [(para, 1.0)]

        graph = defaultdict(lambda: defaultdict(float))
        for i, (para1, words1) in enumerate(paragraph_vectors):
            for j, (para2, words2) in enumerate(paragraph_vectors):
                if i >= j:
                    continue
                intersection = len(words1 & words2)
                union = len(words1 | words2)
                if union > 0:
                    similarity = intersection / union
                    graph[para1][para2] = similarity
                    graph[para2][para1] = similarity

        scores = self.pagerank(graph)

        if not scores:
            scores = {para: 1.0 for para, _ in paragraph_vectors}

        query_boosted_scores = {}
        for para, score in scores.items():
            words = set(self.tokenize(para))
            keyword_match = len(words & query_keywords)
            query_in_para = 1.0 if query.lower() in para.lower() else 0.0
            query_boosted_scores[para] = score * (1 + 0.3 * keyword_match + 0.5 * query_in_para)

        ranked = sorted(query_boosted_scores.items(), key=lambda x: x[1], reverse=True)
        return ranked[:top_n]


# ============================================================================
# Main Tool Function
# ============================================================================

def websearch(query: str, num_results: int = 5, crawl_top_n: int = 3, snippets_per_page: int = 5) -> dict:
    """
    Main web search function with content crawling.

    Args:
        query: Search query string
        num_results: Number of search results to fetch
        crawl_top_n: Number of top results to crawl for content
        snippets_per_page: Number of snippets to extract per page

    Returns:
        Dictionary with search results and extracted content
    """
    print(f"Searching for: {query}", file=sys.stderr)

    # Step 1: Perform search using Baidu
    search_result = baidu_search(query, num_results)

    if not search_result.get('success'):
        return search_result

    if not search_result.get('results'):
        return {
            "success": True,
            "query": query,
            "results": [],
            "message": "No search results found."
        }

    print(f"Found {len(search_result['results'])} results", file=sys.stderr)

    # Filter out results with invalid URLs (missing host)
    from urllib.parse import urlparse
    valid_results = []
    for r in search_result['results']:
        try:
            parsed = urlparse(r.get('url', ''))
            if parsed.scheme and parsed.netloc:
                valid_results.append(r)
            else:
                print(f"Filtering out invalid URL (missing host): {r.get('url', '')}", file=sys.stderr)
        except Exception:
            print(f"Filtering out invalid URL (parse error): {r.get('url', '')}", file=sys.stderr)
    search_result['results'] = valid_results

    if not search_result['results']:
        print(json.dumps({
            "success": False,
            "message": "No valid search results found after URL validation."
        }), file=sys.stderr)
        return

    # Step 2: Rank search results using Lexrank
    lexrank = Lexrank()
    titles = [r['title'] for r in search_result['results']]
    ranked_results = lexrank.rank(titles, query, top_n=crawl_top_n)

    

    print(f"Ranked results, crawling top {len(ranked_results)} pages...", file=sys.stderr)

    # Step 3: Crawl top-ranked pages and extract content
    crawled_content = []

    for rank_idx, (result_idx, score) in enumerate(ranked_results):
        result = search_result['results'][result_idx]
        url = result['url']

        print(f"- [{rank_idx + 1}/{len(ranked_results)}] Crawling: {url[:60]}...\n", file=sys.stdout)

        title, content = fetch_page_content(url)

        if title and content:
            textrank = TextRank()
            ranked_sentences = textrank.rank_sentences(content, query, top_n=snippets_per_page)

            snippets = [sentence for sentence, score in ranked_sentences if sentence]

            crawled_content.append({
                "url": url,
                "page_title": title,
                "search_title": result['title'],
                "search_snippet": result.get('snippet', ''),
                "content_snippets": snippets,
                "full_content": content[:10000]
            })

            print(f"  Extracted {len(snippets)} snippets ({len(content)} chars)", file=sys.stderr)
        else:
            print(f"  Failed to extract content", file=sys.stderr)
            crawled_content.append({
                "url": url,
                "page_title": title or "Unknown",
                "search_title": result['title'],
                "search_snippet": result.get('snippet', ''),
                "content_snippets": [],
                "error": "Failed to extract content"
            })

    return {
        "success": True,
        "query": query,
        "search_results": search_result['results'],
        "crawled_content": crawled_content,
        "total_crawled": len(crawled_content)
    }


def main():
    """Main entry point for the websearch tool."""
    # Set UTF-8 encoding for stdout
    sys.stdout.reconfigure(encoding='utf-8') if hasattr(sys.stdout, 'reconfigure') else None
    
    parser = argparse.ArgumentParser(
        description='Web Search Tool with Selenium-based Content Crawling',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Examples:
  python main.py -query "Python programming"
        """
    )

    parser.add_argument(
        '-query',
        type=str,
        required=True,
        help='Search query string. 使用重点关键词搜索能提升搜索效果。'
    )
    parser.add_argument(
        '-debug',
        action='store_true',
        help='Enable debug mode to print exceptions to stdout (default: False)'
    )

    args = parser.parse_args()

    try:
        result = websearch(
            query=args.query,
            num_results=10,
            crawl_top_n=6,
            snippets_per_page=20
        )
        print("## Websearch Result:\n\n", file=sys.stdout)
        print("\n".join(["\n".join([snip for snip in "".join(page.get("content_snippets")).split("\n") if len(snip)>20]) for page in result.get("crawled_content", [])]) + "\n\n---\n", file=sys.stdout)
    except Exception as e:
        if args.debug:
            import traceback
            print(f"Exception: {type(e).__name__}: {str(e)}", file=sys.stderr)
            print(traceback.format_exc(), file=sys.stderr)
        raise
    finally:
        close_chrome_driver()


if __name__ == '__main__':
    main()
