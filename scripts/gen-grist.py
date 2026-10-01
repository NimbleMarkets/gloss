#!/usr/bin/env python3
"""Regenerate the original Grist fixtures: python3 scripts/gen-grist.py

examples/notes.grist is laid out as Grist lays a document out, and
internal/document/testdata/plain.sqlite is a SQLite database that is not a
Grist document, for the tests.

The catalog tables carry Grist's own definitions; only those gloss reads are
here, so Grist itself would not take this file for a whole document. Python
writes the files because its SQLite leaves pages whole to the last byte, as
Grist's does; some builds of the sqlite3 command hold a few bytes of each back.
"""
import marshal
import os
import sqlite3

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

NOTES = """
CREATE TABLE "_grist_DocInfo" (id INTEGER PRIMARY KEY, "docId" BLOB DEFAULT '', "peers" BLOB DEFAULT '', "basketId" BLOB DEFAULT '', "schemaVersion" BLOB DEFAULT 0, "timezone" BLOB DEFAULT '', "documentSettings" TEXT DEFAULT '');
CREATE TABLE "_grist_Tables" (id INTEGER PRIMARY KEY, "tableId" BLOB DEFAULT '', "primaryViewId" BLOB DEFAULT 0, "summarySourceTable" BLOB DEFAULT 0, "onDemand" BLOB DEFAULT 0, "rawViewSectionRef" INTEGER DEFAULT 0, "recordCardViewSectionRef" INTEGER DEFAULT 0);
CREATE TABLE "_grist_Tables_column" (id INTEGER PRIMARY KEY, "parentId" BLOB DEFAULT 0, "parentPos" BLOB DEFAULT 1e999, "colId" BLOB DEFAULT '', "type" BLOB DEFAULT '', "widgetOptions" BLOB DEFAULT '', "isFormula" BLOB DEFAULT 0, "formula" BLOB DEFAULT '', "label" BLOB DEFAULT '', "untieColIdFromLabel" BLOB DEFAULT 0, "summarySourceCol" BLOB DEFAULT 0, "displayCol" BLOB DEFAULT 0, "visibleCol" BLOB DEFAULT 0, "recalcWhen" INTEGER DEFAULT 0, "recalcDeps" BLOB DEFAULT NULL, "rules" TEXT DEFAULT NULL, "description" TEXT DEFAULT '', "reverseCol" INTEGER DEFAULT 0);
CREATE TABLE "_grist_Views_section" (id INTEGER PRIMARY KEY, "tableRef" BLOB DEFAULT 0, "parentId" BLOB DEFAULT 0, "parentKey" BLOB DEFAULT '', "title" BLOB DEFAULT '', "defaultWidth" BLOB DEFAULT 0, "borderWidth" BLOB DEFAULT 0, "theme" BLOB DEFAULT '', "options" BLOB DEFAULT '', "chartType" BLOB DEFAULT '', "layoutSpec" BLOB DEFAULT '', "filterSpec" BLOB DEFAULT '', "sortColRefs" BLOB DEFAULT '', "linkSrcSectionRef" BLOB DEFAULT 0, "linkSrcColRef" BLOB DEFAULT 0, "linkTargetColRef" BLOB DEFAULT 0, "embedId" BLOB DEFAULT '', "rules" TEXT DEFAULT NULL, "description" TEXT DEFAULT '', "shareOptions" TEXT DEFAULT '');
CREATE TABLE "_grist_Attachments" (id INTEGER PRIMARY KEY, "fileIdent" BLOB DEFAULT '', "fileName" BLOB DEFAULT '', "fileType" BLOB DEFAULT '', "fileSize" BLOB DEFAULT 0, "imageHeight" BLOB DEFAULT 0, "imageWidth" BLOB DEFAULT 0, "timeUploaded" BLOB DEFAULT NULL, "timeDeleted" DATETIME DEFAULT NULL, "fileExt" TEXT DEFAULT '');

INSERT INTO _grist_DocInfo (id, schemaVersion, timezone) VALUES (1, 46, 'UTC');

-- The tables: three of the user's, one of them empty, and a summary.
INSERT INTO _grist_Tables (id, tableId, summarySourceTable, rawViewSectionRef) VALUES
  (1, 'Sightings', 0, 1),
  (2, 'Sites', 0, 2),
  (3, 'Ideas', 0, 3),
  (4, 'Sightings_summary_Site', 1, 4);
INSERT INTO _grist_Views_section (id, tableRef, parentKey, title) VALUES
  (1, 1, 'record', 'Field sightings'),
  (2, 2, 'record', ''),
  (3, 3, 'record', 'Ideas for next season'),
  (4, 4, 'record', '');

-- Their columns, in the order Grist shows them, which is parentPos and not
-- the order SQLite holds them in.
INSERT INTO _grist_Tables_column (id, parentId, parentPos, colId, type, label, isFormula, formula, displayCol, visibleCol, widgetOptions) VALUES
  (1, 1, 1, 'manualSort', 'ManualSortPos', 'manualSort', 0, '', 0, 0, ''),
  (2, 1, 2, 'Species', 'Text', 'Species', 0, '', 0, 0, ''),
  (3, 1, 3, 'Count', 'Int', 'How many', 0, '', 0, 0, ''),
  (4, 1, 4, 'Weight_kg', 'Numeric', 'Weight (kg)', 0, '', 0, 0, ''),
  (5, 1, 5, 'Confirmed', 'Bool', 'Confirmed?', 0, '', 0, 0, ''),
  (6, 1, 6, 'Seen', 'Date', 'Seen on', 0, '', 0, 0, ''),
  (7, 1, 7, 'Logged', 'DateTime:UTC', 'Logged at', 0, '', 0, 0, ''),
  (8, 1, 2.5, 'Site', 'Ref:Sites', 'Site', 0, '', 12, 15, ''),
  (9, 1, 9, 'Tags', 'ChoiceList', 'Tags', 0, '', 0, 0, ''),
  (10, 1, 10, 'Source', 'Text', 'Source', 0, '', 0, 0, '{"widget":"HyperLink"}'),
  (11, 1, 11, 'Per_kg', 'Any', 'Count per kg', 1, '$Count / $Weight_kg', 0, 0, ''),
  (12, 1, 12, 'gristHelper_Display', 'Any', 'gristHelper_Display', 1, '$Site.Name', 0, 0, ''),
  (13, 1, 13, 'Photos', 'Attachments', 'Photos', 0, '', 0, 0, ''),
  (14, 2, 1, 'manualSort', 'ManualSortPos', 'manualSort', 0, '', 0, 0, ''),
  (15, 2, 2, 'Name', 'Text', 'Site name', 0, '', 0, 0, ''),
  (16, 2, 3, 'Region', 'Choice', 'Region', 0, '', 0, 0, ''),
  (17, 2, 4, 'Elevation_m', 'Int', 'Elevation (m)', 0, '', 0, 0, ''),
  (18, 3, 1, 'manualSort', 'ManualSortPos', 'manualSort', 0, '', 0, 0, ''),
  (19, 3, 2, 'Idea', 'Text', 'Idea', 0, '', 0, 0, ''),
  (20, 3, 3, 'Owner', 'Text', 'Who', 0, '', 0, 0, ''),
  (21, 4, 1, 'Site', 'Ref:Sites', 'Site', 0, '', 0, 0, ''),
  (22, 4, 2, 'count', 'Int', 'count', 1, 'len($group)', 0, 0, ''),
  (23, 4, 1.5, 'group', 'RefList:Sightings', 'group', 1, 'table.getSummarySourceGroup(rec)', 0, 0, '');

INSERT INTO _grist_Attachments (id, fileIdent, fileName, fileType, fileSize) VALUES
  (1, '0000000000000000000000000000000000000000.png', 'heron.png', 'image/png', 0);

CREATE TABLE "Sightings" (id INTEGER PRIMARY KEY, "manualSort" NUMERIC DEFAULT 1e999, "Species" TEXT DEFAULT '', "Count" INTEGER DEFAULT 0, "Weight_kg" NUMERIC DEFAULT 0, "Confirmed" BOOLEAN DEFAULT 0, "Seen" DATE DEFAULT NULL, "Logged" DATETIME DEFAULT NULL, "Site" INTEGER DEFAULT 0, "Tags" TEXT DEFAULT NULL, "Source" TEXT DEFAULT '', "Per_kg" BLOB DEFAULT NULL, "gristHelper_Display" BLOB DEFAULT NULL, "Photos" TEXT DEFAULT NULL);
CREATE TABLE "Sites" (id INTEGER PRIMARY KEY, "manualSort" NUMERIC DEFAULT 1e999, "Name" TEXT DEFAULT '', "Region" TEXT DEFAULT '', "Elevation_m" INTEGER DEFAULT 0);
CREATE TABLE "Ideas" (id INTEGER PRIMARY KEY, "manualSort" NUMERIC DEFAULT 1e999, "Idea" TEXT DEFAULT '', "Owner" TEXT DEFAULT '');
CREATE TABLE "Sightings_summary_Site" (id INTEGER PRIMARY KEY, "Site" INTEGER DEFAULT 0, "count" INTEGER DEFAULT 0, "group" TEXT DEFAULT NULL);

-- manualSort puts the otter first, as a row dragged above the heron would be.
-- The dragonfly's date is text Grist could not read as a date, it has no
-- site, and its formula divides by nothing: Grist keeps the error marshalled.
-- Source is a hyperlink column: words, then the address they lead to.
INSERT INTO Sightings (id, manualSort, Species, Count, Weight_kg, Confirmed, Seen, Logged, Site, Tags, Source, Per_kg, gristHelper_Display, Photos) VALUES
  (1, 2, 'Grey heron', 2, 1.8, 1, 1772841600, 1772881500, 1, '["wading","dawn"]', 'Heron notes https://example.com/heron', 1.25, 'Reed Marsh', '[1]'),
  (2, 1, 'Otter', 1, 8, 1, 1773014400, 1773020000.5, 2, '["mammal"]', '', 0.125, 'Alder Creek', NULL),
  (3, 3, 'Emperor dragonfly', 14, 0, 0, 'early March', NULL, 0, NULL, '', X'{error}', NULL, NULL),
  (4, 4, 'Kingfisher', 1, 0.04, 1, 1773446400, 1773480000, 1, '[]', 'https://example.com/kingfisher', 25, 'Reed Marsh', NULL);
INSERT INTO Sites (id, manualSort, Name, Region, Elevation_m) VALUES
  (1, 1, 'Reed Marsh', 'Lowland', 12),
  (2, 2, 'Alder Creek', 'Upland', 340);
INSERT INTO Sightings_summary_Site (id, Site, count, "group") VALUES (1, 1, 2, '[1,4]'), (2, 2, 1, '[2]'), (3, 0, 1, '[3]');
"""

PLAIN = """
CREATE TABLE words (id INTEGER PRIMARY KEY, word TEXT);
INSERT INTO words (word) VALUES ('heron'), ('otter');
"""


def write(path, script):
    path = os.path.join(ROOT, path)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    if os.path.exists(path):
        os.remove(path)
    db = sqlite3.connect(path)
    db.execute("PRAGMA page_size = 1024")
    db.executescript(script)
    db.commit()
    db.execute("VACUUM")
    db.close()
    print(os.path.getsize(path), path)


# Grist marshals what is not a plain value, in the second version of the format.
error = marshal.dumps(["E", "ZeroDivisionError"], 2).hex()
write("examples/notes.grist", NOTES.replace("{error}", error))
write("internal/document/testdata/plain.sqlite", PLAIN)
