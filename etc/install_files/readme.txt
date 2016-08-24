Exit 66 JukeBox
Copyright (c) 2001-2011, Andrew Barilla
http://www.exit66.com/

Description:
Exit 66 JukeBox is a web based jukebox program for MP3 and OGG files.  It runs 
silently in the background and can be accessed by any machine on your local 
network.

Quick Start:
After installation, start up Exit 66 JukeBox as you would any other program.  
It will run silently in the background and can be accessed by a web browser 
from any machine on your network.

If this is your first time running Exit 66 Jukebox, right click on the icon in 
the system tray and select Launch Exit 66 Jukebox or you can use your web browser 
to go to http://localhost.

In the toolbar across the top of the page, click the Gear button.  A library 
is a directory or network share where you have your MP3s stored, C:\Audio for 
example.  Enter the path into the empty text box.  

Press the 'Add' button.  The scanning process will begin immediately.  This 
process will run in the background and as new files are added you will see 
them appear in the JukeBox.  Be patient as this may take a while.  You may
have to refresh the browser to see the latest items.

Now you can click 'Artists' and request MP3 files to be played by the 
JukeBox.

For more details on how to take full advantage of Exit 66 JukeBox please check 
out the help file.

This software utilizes the following libraries:
- JOrbis (http://www.jcraft.com/jorbis/)
- hsqldb (http://hsqldb.sourceforge.net/)
- SysTray for Java (http://systray.sourceforge.net/)
- Jetty (http://www.eclipse.org/jetty/)
- MyID3 (http://www.fightingquaker.com/myid3/)

Changelog:

23 June 2011 - v.5.0.0
------------------------------------
-Changed: Everything!  This is a complete rewrite.  Your old database will be ignored
and a new one will be created.

26 Apr 2006 - v.3.0.7
------------------------------------
-Fixed: Issue with album cover images not loading correctly

25 Apr 2006 - v.3.0.6
------------------------------------
-Fixed: Various speed enhancements

24 Apr 2006 - v.3.0.5
------------------------------------
-Fixed: bugs when viewing in Internet Explorer
-Added: console only mode by passing "console" through command line
-Changed: switched to using Jetty as the webserver instead of the original home grown server

19 Apr 2006 - v.3.0.beta.4
------------------------------------
-Added: Playlist rules: can define what tracks to play by library, track or genre
-Fixed: files with uppercase extensions not importing

17 Apr 2006 - v.3.0.beta.3
------------------------------------
-Added: Ability to play/pause
-Added: Password protect admin area - any user name works, no password by default
-Added: Caching of album image covers for enhanced performance
-Changed: Unknown Albums will be unique per artist
-Changed: Search results returns 50 results at a time and can be paged through
-Changed: Enhanced performance of artist list for large number of artists
-Changed: Began conversion of admin area to new look and feel
-Changed: Upgraded all components to latest versions
-Fixed: Display problem in browsers with long lists

26 Oct 2005 - v.3.0.beta.2
------------------------------------
-Fixed: Problem with import of tracks from albums with multiple artists
-Changed: The program and system tray icon

25 Oct 2005 - v.3.0.beta.1
------------------------------------
-Added: Completely new interface
-Added: XML REST architecture responses for use with AJAX techniques
-Removed: Socket server, can now use XML responses instead for external 
 applications
-Fixed: Use artist order name (i.e. ignore 'The') to find an existing artist
-Fixed: Same album being inserted twice as two different albums during import
-Fixed: Bug in tritonus code which resulted in certain MP3 files not being 
 played
-Enhanced: Scanning a library now returns immediately instead of searching for
 files first
-Enhanced: Improved speed through the use of indexes and various other items

09 Aug 2005 - v.2.7.0
------------------------------------
-Added: System tray icon for Windows
-Upgraded: License information to the GPL
-Upgraded: Enhanced the request links and currently playing text to use 
 AJAX (like GMail)

04 Mar 2005 - v.2.6.1
------------------------------------
-Fixed: Stop Current Song now works
-Fixed: Installer didn't set working directory for shortcuts (special thanks to 
 Polychronopolis for picking up on this)

27 Jan 2005 - v.2.6.0
------------------------------------
-Renamed from BlueVade Jukebox to Exit 66 Jukebox
-Added: Support for OGG files
-Upgraded: To Java 5 for better support of file tags

28 Jan 2004 - v2.5.0b
------------------------------------
-Added: GIFs are now supported when album images are pulled from directories 
-Added: BlueVade Jukebox Kiosk - a full screen graphical frontend

22 Jan 2004 - v2.0.3
------------------------------------
-Fixed: ID3 tag reader can handle track numbers in the format ##/##
-Fixed: ID3 tag reader strips null characters and whitespaces
-Fixed: Rescan all tracks function was only scanning for new tracks
-Fixed: ID3 tags not read correctly for MusicMatch tagged files
-Added: new templates default, default_no_admin, default_no_pics, 
 default_no_pics_or_admin.  Templates can be accessed by 
 http://localhost/default_no_admin/
-Added: usealbumimagefile to options file.  Setting this to 1 allows the system 
 to search a directory for an image to use as an album cover when none is 
 specified

07 Feb 2003 - v2.0.2
------------------------------------
-Fixed: Error with new installations not setting the database version in the 
 option file correctly
-Fixed: Certain ID3 tags caused infinite loops

12 Jun 2002 - v2.0.b3
------------------------------------
-Added request album/artist feature
-Added ability to show cover image from MP3 file (uses track #1's image for 
 entire album image)
-Added nolog command line parameter to turn logging to file off
-System will only scan one library at a time
-Added logging of when scans start and stop to either the system console or the 
 error log file
-Added text 'Scanning...' next to a library in the admin area while the library 
 is being scanned
-Added 'Edit Database' area in Admin to modify artist and album names, i.e. to 
 change the alphabetical sort of Lou Reed to Reed, Lou
-JukeBox won't allow more than once instance of itself to run at a time
-Added link in Admin area to shutdown server
-Added link in Admin area under Libraries to remove missing files from the 
 library
-Fixed: Only files with lower case extensions would import
-Added list of files that failed to import in the Library area 

26 Nov 2002 - v2.0.b2
------------------------------------
-Added error logging to text file
-Fixed form processing problem with Internet Explorer
-Modified scan to only import new tracks
-Added scanall feature to import new tracks and rescan imported tracks

18 Nov 2002 - v2.0.b1
------------------------------------
-Complete rewrite in Java
	+Cross-platform
	+Telnet server
	+Template driven webserver for customized interfaces
-Activate more than one playlist at a time
-Remove individual tracks from queue
-Empty entire queue
-Split albums up by artist for albums with same names but different artists 

3 Oct 2002 - v1.1.0
-----------------------------------
-Who knows?  The notes have been lost

4 Jul 2002 - v1.0.0
-----------------------------------
-Initial Public Release