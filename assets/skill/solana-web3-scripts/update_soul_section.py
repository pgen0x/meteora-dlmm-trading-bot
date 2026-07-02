#!/usr/bin/env python3
"""
update_soul_section.py — Safely update a specific section of SOUL.md.
Creates a backup of SOUL.md before making any changes.

Usage:
  python3 update_soul_section.py <SECTION_NUMBER> [NEW_CONTENT_FILE_OR_STRING]
  OR
  cat new_content.txt | python3 update_soul_section.py <SECTION_NUMBER>
"""

import sys
import os
import shutil

# Resolved from this file's own location (<profile>/skills/solana-web3/scripts/) so the
# script works whether it's a copy or a symlink into a Hermes profile.
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROFILE_DIR = os.path.dirname(os.path.dirname(os.path.dirname(SCRIPT_DIR)))
SOUL_PATH = os.path.join(PROFILE_DIR, "SOUL.md")
BACKUP_PATH = os.path.join(PROFILE_DIR, "SOUL.md.bak")


def main():
    if len(sys.argv) < 2:
        print("Usage: python3 update_soul_section.py <SECTION_NUMBER> [NEW_CONTENT_FILE_OR_STRING]")
        sys.exit(1)

    section_num = sys.argv[1].strip()
    new_content = ""

    # Read new content from file, argument, or stdin
    if len(sys.argv) >= 3:
        arg_val = sys.argv[2]
        if os.path.exists(arg_val):
            with open(arg_val, "r", encoding="utf-8") as f:
                new_content = f.read()
        else:
            new_content = arg_val
    else:
        # Read from stdin
        if not sys.stdin.isatty():
            new_content = sys.stdin.read()
        else:
            print("Error: No new content provided via argument or stdin.")
            sys.exit(1)

    new_content = new_content.strip()
    if not new_content:
        print("Error: New content is empty.")
        sys.exit(1)

    if not os.path.exists(SOUL_PATH):
        print(f"Error: SOUL.md not found at {SOUL_PATH}")
        sys.exit(1)

    # 1. Create a backup of SOUL.md first
    try:
        shutil.copy2(SOUL_PATH, BACKUP_PATH)
        print(f"Created backup at {BACKUP_PATH}")
    except Exception as e:
        print(f"Error creating backup: {e}")
        sys.exit(1)

    # 2. Read SOUL.md content
    with open(SOUL_PATH, "r", encoding="utf-8") as f:
        soul_content = f.read()

    lines = soul_content.splitlines()
    updated_lines = []
    
    in_target_section = False
    replaced = False
    
    target_header_prefix = f"## {section_num}."

    for line in lines:
        stripped = line.strip()
        
        # Check if we are hitting the target section header
        if stripped.startswith(target_header_prefix):
            in_target_section = True
            # We append the header, then insert the new content (minus the header itself if it was included in new_content)
            updated_lines.append(line)
            
            # If the new content already includes the header, don't append it again
            lines_to_add = new_content.splitlines()
            if lines_to_add and lines_to_add[0].strip().startswith(target_header_prefix):
                lines_to_add = lines_to_add[1:]
                
            updated_lines.extend(lines_to_add)
            replaced = True
            continue
            
        # Check if we are exiting the target section
        if in_target_section:
            if stripped.startswith("## ") or stripped == "---":
                in_target_section = False
                # Re-enable adding lines
            else:
                # Skip target section's original content
                continue
                
        # Append lines that are not part of the replaced section content
        updated_lines.append(line)

    if not replaced:
        print(f"Error: Section '{section_num}' not found in SOUL.md (looked for header starting with '{target_header_prefix}')")
        # Restore backup
        shutil.copy2(BACKUP_PATH, SOUL_PATH)
        sys.exit(1)

    # 3. Write modified lines back to SOUL.md
    try:
        with open(SOUL_PATH, "w", encoding="utf-8") as f:
            f.write("\n".join(updated_lines) + "\n")
        print(f"SUCCESS: Updated Section {section_num} in SOUL.md")
    except Exception as e:
        print(f"Error writing to SOUL.md: {e}")
        # Restore backup
        shutil.copy2(BACKUP_PATH, SOUL_PATH)
        sys.exit(1)


if __name__ == "__main__":
    main()
