#!/usr/bin/env python3
"""Generate the realistic, fully fictional invoice fixtures used by the demo.

These files are the source panels the review screen renders and the inputs the
offline fake extractor is keyed to. Every value here is invented; none of the
suppliers, emails, or numbers refer to a real business.

Three documents exercise the three real pipeline paths:

  * fixture-aurora-stationery.pdf   text PDF -> pdftotext, extracts cleanly,
                                    normalizes with NO server warnings.
  * fixture-meridian-supplies.png   raster image -> Tesseract OCR path, also
                                    clean. The OCR marker is a bare
                                    letters/digits/hyphen token so a pinned
                                    Tesseract reads it reliably.
  * fixture-cedarline-services.pdf  text PDF whose printed subtotal + tax does
                                    not equal its printed total, so the server
                                    emits exactly one subtotal_tax_total_mismatch
                                    warning through the real normalizer.

Each document embeds a machine-readable marker string. The fake extractor
(internal/extraction/fake.go) matches a document only when BOTH the committed
SHA-256 and this marker are present, so the marker must survive extraction/OCR.

Regeneration (the committed files, not this script, are what the app uses):

    python3 -m venv .venv && . .venv/bin/activate
    pip install "reportlab>=4,<6" "pillow>=10"
    python3 scripts/gen-fixtures.py

Then run `go test ./cmd/worker/...`: it recomputes each file's SHA-256 and fails
if the constants in defaultFakeFixtures() no longer match, printing the new
hashes to paste back. The bytes are deterministic for a given reportlab/Pillow
version (reportlab invariant mode fixes timestamps); a version bump can change
the hash, which is expected and caught by that test.
"""

from __future__ import annotations

import hashlib
import os
from pathlib import Path

os.environ.setdefault("SOURCE_DATE_EPOCH", "1717200000")  # 2024-06-01T00:00:00Z

from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.pdfgen import canvas
from PIL import Image, ImageDraw, ImageFont

TESTDATA = Path(__file__).resolve().parent.parent / "testdata"

# Candidate sans-serif TrueType fonts. The first that exists is used; Arial on
# macOS and DejaVuSans on Debian/CI both render crisply enough for OCR.
FONT_CANDIDATES = [
    "/System/Library/Fonts/Supplemental/Arial.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    "/Library/Fonts/Arial.ttf",
]
FONT_BOLD_CANDIDATES = [
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/Library/Fonts/Arial Bold.ttf",
]


def _first_existing(paths: list[str]) -> str:
    for path in paths:
        if Path(path).exists():
            return path
    raise SystemExit(
        "no sans-serif TrueType font found; install DejaVu "
        "(fonts-dejavu-core) or run on macOS with Arial available"
    )


def draw_pdf(path: Path, supplier: str, email: str, number: str,
             issue: str, due: str, marker: str, rows: list[tuple[str, str, str, str]],
             subtotal: str, tax_label: str, tax: str, total: str) -> None:
    c = canvas.Canvas(str(path), pagesize=A4, invariant=1)
    c.setTitle("Fictional sample invoice")
    c.setAuthor("InvoiceFlow demo fixtures")
    c.setSubject("Fictional data only")
    width, height = A4
    left = 20 * mm
    top = height - 25 * mm

    c.setFont("Helvetica-Bold", 20)
    c.drawString(left, top, supplier)
    c.setFont("Helvetica", 10)
    c.drawString(left, top - 7 * mm, email)
    c.drawString(left, top - 12 * mm, "123 Example Way, Springfield, Fictional State")

    c.setFont("Helvetica-Bold", 14)
    c.drawRightString(width - left, top, "INVOICE")
    c.setFont("Helvetica", 10)
    c.drawRightString(width - left, top - 7 * mm, f"Invoice number: {number}")
    c.drawRightString(width - left, top - 12 * mm, f"Issue date: {issue}")
    c.drawRightString(width - left, top - 17 * mm, f"Due date: {due}")

    c.setFont("Helvetica", 10)
    c.drawString(left, top - 30 * mm, "Bill to: Fictional Buyer Ltd, 9 Placeholder Road, Demo City")

    table_top = top - 42 * mm
    c.setFont("Helvetica-Bold", 10)
    c.drawString(left, table_top, "Description")
    c.drawRightString(left + 110 * mm, table_top, "Qty")
    c.drawRightString(left + 140 * mm, table_top, "Unit price")
    c.drawRightString(width - left, table_top, "Line total")
    c.line(left, table_top - 2 * mm, width - left, table_top - 2 * mm)

    c.setFont("Helvetica", 10)
    y = table_top - 9 * mm
    for description, qty, unit, line_total in rows:
        c.drawString(left, y, description)
        c.drawRightString(left + 110 * mm, y, qty)
        c.drawRightString(left + 140 * mm, y, unit)
        c.drawRightString(width - left, y, line_total)
        y -= 7 * mm

    y -= 4 * mm
    c.line(left + 100 * mm, y + 4 * mm, width - left, y + 4 * mm)
    c.drawRightString(left + 140 * mm, y, "Subtotal")
    c.drawRightString(width - left, y, subtotal)
    y -= 6 * mm
    c.drawRightString(left + 140 * mm, y, tax_label)
    c.drawRightString(width - left, y, tax)
    y -= 6 * mm
    c.setFont("Helvetica-Bold", 11)
    c.drawRightString(left + 140 * mm, y, "Total")
    c.drawRightString(width - left, y, total)

    c.setFont("Helvetica-Oblique", 8)
    c.drawString(left, 18 * mm, "INVOICEFLOW FICTIONAL SAMPLE — not a real invoice.")
    c.drawString(left, 14 * mm, marker)
    c.showPage()
    c.save()


def draw_image(path: Path, supplier: str, email: str, number: str,
               issue: str, due: str, marker_line: str,
               rows: list[tuple[str, str, str, str]],
               subtotal: str, tax_label: str, tax: str, total: str) -> None:
    # A4 at ~250 DPI: 2066 x 2924, comfortably under the 40,000,000-pixel intake
    # ceiling while leaving Tesseract crisp glyph edges.
    scale = 2.5
    width, height = int(827 * scale), int(1169 * scale)
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)
    regular = _first_existing(FONT_CANDIDATES)
    bold = _first_existing(FONT_BOLD_CANDIDATES)

    def font(size: int, heavy: bool = False) -> ImageFont.FreeTypeFont:
        return ImageFont.truetype(bold if heavy else regular, int(size * scale))

    black = (17, 17, 17)
    left = int(60 * scale)
    right = width - left

    draw.text((left, int(60 * scale)), supplier, font=font(30, True), fill=black)
    draw.text((left, int(100 * scale)), email, font=font(15), fill=black)
    draw.text((left, int(124 * scale)), "500 Sample Avenue, Fictionville", font=font(15), fill=black)

    def rtext(x_right: int, y: int, text: str, f: ImageFont.FreeTypeFont) -> None:
        w = draw.textlength(text, font=f)
        draw.text((x_right - w, y), text, font=f, fill=black)

    rtext(right, int(60 * scale), "INVOICE", font(24, True))
    rtext(right, int(102 * scale), f"Invoice: {number}", font(16))
    rtext(right, int(126 * scale), f"Issue date: {issue}", font(16))
    rtext(right, int(150 * scale), f"Due date: {due}", font(16))

    draw.text((left, int(200 * scale)),
              "Bill to: Fictional Buyer Ltd, 9 Placeholder Road, Demo City",
              font=font(15), fill=black)

    table_top = int(250 * scale)
    draw.text((left, table_top), "Description", font=font(16, True), fill=black)
    rtext(int(560 * scale), table_top, "Qty", font(16, True))
    rtext(int(680 * scale), table_top, "Unit price", font(16, True))
    rtext(right, table_top, "Line total", font(16, True))
    draw.line([(left, table_top + int(26 * scale)), (right, table_top + int(26 * scale))],
              fill=black, width=max(1, int(scale)))

    y = table_top + int(44 * scale)
    for description, qty, unit, line_total in rows:
        draw.text((left, y), description, font=font(15), fill=black)
        rtext(int(560 * scale), y, qty, font(15))
        rtext(int(680 * scale), y, unit, font(15))
        rtext(right, y, line_total, font(15))
        y += int(30 * scale)

    y += int(20 * scale)
    rtext(int(680 * scale), y, "Subtotal", font(15))
    rtext(right, y, subtotal, font(15))
    y += int(26 * scale)
    rtext(int(680 * scale), y, tax_label, font(15))
    rtext(right, y, tax, font(15))
    y += int(26 * scale)
    rtext(int(680 * scale), y, "Total", font(17, True))
    rtext(right, y, total, font(17, True))

    draw.text((left, height - int(90 * scale)),
              "INVOICEFLOW FICTIONAL SAMPLE - not a real invoice.",
              font=font(14), fill=black)
    # Rendered large and isolated so a pinned Tesseract reads it verbatim.
    draw.text((left, height - int(64 * scale)), marker_line, font=font(20, True), fill=black)

    image.save(path, format="PNG", optimize=True)


def main() -> None:
    TESTDATA.mkdir(exist_ok=True)

    draw_pdf(
        TESTDATA / "fixture-aurora-stationery.pdf",
        supplier="Aurora Stationery Co.",
        email="billing@aurora-stationery.example",
        number="AURORA-1042",
        issue="2026-06-15", due="2026-07-15",
        marker="INVOICEFLOW_FIXTURE:AURORA-1042",
        rows=[
            ("A4 copy paper, 80 gsm (5 reams)", "5", "6.00", "30.00"),
            ("Gel ink pens, box of 12", "3", "8.00", "24.00"),
            ("Mesh desk organizer", "2", "13.00", "26.00"),
        ],
        subtotal="80.00", tax_label="Tax (8%)", tax="6.40", total="86.40",
    )

    draw_image(
        TESTDATA / "fixture-meridian-supplies.png",
        supplier="Meridian Office Supplies",
        email="accounts@meridian-supplies.example",
        number="MERIDIAN-2087",
        issue="2026-06-18", due="2026-07-18",
        marker_line="Reference MERIDIAN-2087",
        rows=[
            ("Ballpoint pens, box of 50", "4", "9.00", "36.00"),
            ("Sticky notes, pack of 12", "6", "4.50", "27.00"),
        ],
        subtotal="63.00", tax_label="Tax (8%)", tax="5.04", total="68.04",
    )

    draw_pdf(
        TESTDATA / "fixture-cedarline-services.pdf",
        supplier="Cedarline Services LLC",
        email="ar@cedarline.example",
        number="CEDAR-3390",
        issue="2026-06-20", due="2026-07-20",
        marker="INVOICEFLOW_FIXTURE:CEDAR-3390",
        rows=[
            ("Managed hosting, monthly", "10", "15.00", "150.00"),
            ("On-site support, hours", "4", "25.00", "100.00"),
        ],
        # Printed subtotal + tax (250.00 + 47.50 = 297.50) does not equal the
        # printed total (290.00): the human-in-the-loop warning fixture.
        subtotal="250.00", tax_label="Tax", tax="47.50", total="290.00",
    )

    for name in sorted(p.name for p in TESTDATA.glob("fixture-*")):
        digest = hashlib.sha256((TESTDATA / name).read_bytes()).hexdigest()
        print(f"{digest}  testdata/{name}")


if __name__ == "__main__":
    main()
