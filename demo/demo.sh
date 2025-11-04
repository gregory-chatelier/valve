#!/bin/bash

# This script demonstrates the core features of the 'valve' command.

set -e

echo "--- Valve Demo Script ---"

# Ensure valve is in the PATH or in the current directory
if ! command -v valve &> /dev/null; then
    if [ -f ./valve ]; then
        alias valve='./valve'
    else
        echo "'valve' command not found. Please build it or add it to your PATH."
        exit 1
    fi
fi

# --- Demo 1: Basic Line-Based Rate Limiting ---
echo "
--- 1. Limiting output to 2 lines/sec for 3 seconds ---"
(for i in $(seq 1 10); do echo "Line $i"; sleep 0.1; done) | valve --rate 2/s | head -n 6

# --- Demo 2: Byte-Based Rate Limiting with Progress ---
echo "
--- 2. Limiting data transfer to 5MB/s with a progress bar ---"
echo "Creating a 10MB dummy file..."
head -c 10M /dev/urandom > dummy.bin
echo "Piping file through valve. This should take ~2 seconds."
cat dummy.bin | valve --rate 5MB/s --progress | wc -c

# --- Demo 3: Simulating API Call Throttling ---
echo "
--- 3. Simulating API calls at a rate of 3 per second ---"
echo "User ID 1" > user_ids.txt
echo "User ID 2" >> user_ids.txt
echo "User ID 3" >> user_ids.txt
echo "User ID 4" >> user_ids.txt
echo "User ID 5" >> user_ids.txt

cat user_ids.txt | valve --rate 3/s | while read -r id; do 
  echo "Processing $id at $(date +%H:%M:%S.%N)"; 
done

# --- Demo 4: Long-Term Rate Spacing ---
echo "
--- 4. Spacing out 6 events over 30 seconds (rate of 12/m) ---"
seq 1 6 | valve --rate 12/m | while read -r i; do 
  echo "Event #$i triggered at $(date +%H:%M:%S)"; 
done

# --- Demo 5: Buffering Strategy (drop-newest) ---
echo "
--- 5. Demonstrating 'drop-newest' buffering strategy ---"
echo "Quickly feeding 10 items into a valve with a buffer size of 2, at a slow output rate of 1/s."
echo "Only the first 2 items should get through; the rest are dropped."
(for i in $(seq 1 10); do echo "Item $i"; done) | valve --rate 1/s --max-buffer 2 --on-full drop-newest

# --- Cleanup ---
echo "
--- Cleaning up dummy files ---"
rm dummy.bin user_ids.txt
echo "Demo complete."
