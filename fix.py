from pathlib import Path

FILES = [
    "handlers.go",
    "service.go",
    "bot.go",
    "models.go",
    "config.go",
    "repository.go",
]

SUS_MARKERS = ("Р", "С", "вЂ", "рџ", "Ð", "Ñ")

def suspicious_score(s: str) -> int:
    return sum(s.count(m) for m in SUS_MARKERS)

def try_fix_mojibake(line: str) -> str:
    # Пробуем классическое восстановление: cp1251 -> utf8
    try:
        fixed = line.encode("cp1251").decode("utf-8")
    except Exception:
        return line

    # Берем фикс только если реально стало лучше
    if suspicious_score(fixed) < suspicious_score(line):
        return fixed
    return line

for name in FILES:
    p = Path(name)
    if not p.exists():
        continue

    raw = p.read_bytes()

    # Читаем как UTF-8 (с BOM если есть)
    text = raw.decode("utf-8", errors="strict")
    bom = "\ufeff" if text.startswith("\ufeff") else ""
    if bom:
        text = text.lstrip("\ufeff")

    lines = text.splitlines(keepends=True)
    fixed_lines = [try_fix_mojibake(line) for line in lines]
    fixed_text = "".join(fixed_lines)

    # Убираем BOM (лучше без него в Go-файлах)
    out = fixed_text

    # Бэкап
    bak = p.with_suffix(p.suffix + ".bak")
    if not bak.exists():
        bak.write_bytes(raw)

    p.write_text(out, encoding="utf-8", newline="")
    print(f"fixed: {p} (backup: {bak})")