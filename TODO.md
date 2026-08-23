# FIXME

## Bugs

- When converting video files, the screen start to flicker, especially the tumbnails, and parts of the screen becomes less interactive.
- The "browse" button still doesn't work as intended. Should open the file explorer in Windows/WSL, finder on Mac, etc.
- When the progressbar is present, the lower part of the window is below the visible screen. Should automatically adjust the spacing or allow me to scroll further down.
- Improve dynamic scalability on smaller screens
- When using dupfinder, show the same "similarity" score between both/all images/videos of the same duplicate group
- "STOP" button doesn't work as intended in DupFinder and Organizer tab.
- When searching a large space for duplicates, the search stops before it finishes. Implement any strategy to continue searching until all files are searched regarless of the number/size of the files.

## Features

- The progressbar should be more compact. Ideally the stop button should be to the right on the same level as the progressbar.
- Allow me to collapse the "Scan folder" window. Also make this section more compact.
- Allow for a seperate selection of video container format (mp4, mkv, webm) and codac (h264, h265, av1, vp9)
- Improve the scalability of the file view. "Results" stay wider than it needs to be in half-window mode, despite having a lot of unused space
- Add a way to sort the results by File(name), Type, and Size
- After a image or video have been converted, the new file should be automatically linked to the preview to inspect the new file.
- Store the state of converted files, removed duplicates, etc. if program stop before finish.
- Support hardware acceleration for video conversion (NVENC, etc.)
