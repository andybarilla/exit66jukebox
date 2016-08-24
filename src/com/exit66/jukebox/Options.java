package com.exit66.jukebox;

import java.io.BufferedReader;
import java.io.DataOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.InetAddress;
import java.text.DateFormat;
import java.util.Calendar;
import java.util.Date;
import java.util.GregorianCalendar;
import java.util.HashMap;
import java.util.Map;
import java.util.Properties;
import java.util.StringTokenizer;
import java.util.TimeZone;

/**
 *
 * A collection of static procedures primarily to get and set options, but also some
 * commonly used global functions are here
 *
 * @author	Andrew Barilla
 * @version	2.0
 *
 */
public class Options {
    
    static Properties options;
    static Map<String, String> _replace = null;
    
    private static final String DEFAULT_WEBSERVERPORT = "80";
    private static final String DEFAULT_WEBDIRECTORY = System.getProperty("user.dir") + System.getProperty("file.separator") + "web";
    private static final String DEFAULT_DEFAULTIMAGE = System.getProperty("user.dir") + System.getProperty("file.separator") + "web" + System.getProperty("file.separator") + "images" + System.getProperty("file.separator") + "noimage.jpg";
    private static final String DEFAULT_CACHEDIRECTORY = System.getProperty("user.dir") + System.getProperty("file.separator") + "cache"  + System.getProperty("file.separator");
    private static final String DEFAULT_UNKNOWNARTIST = "[Unknown Artist]";
    private static final String DEFAULT_UNKNOWNALBUM = "[Unknown Album]";
    private static final String DEFAULT_VARIOUSARTIST = "Various Artists";
    private static final String DEFAULT_DEFAULTFILE = "index.html";
    private static final String DEFAULT_RANDOMCOUNT = "0";
    private static final String DEFAULT_UNKNOWNCONTENT = "[Unclassified]";
    private static final String DEFAULT_SCANTHREADCOUNT = "5";
    private static final String DEFAULT_IGNOREALPHA = "the,el,la,los,las,le,les,die,das,der";
    private static final String DEFAULT_REPLACE = "n : and , 'n : and , n' : and ,&:and";
    private static final String DEFAULT_HISTORYCOUNT = "0";
    private static final String DEFAULT_PLAYLISTHISTORYCOUNT = "10";
    private static final String DEFAULT_DATABASEVERSION = "0";
    private static final String DEFAULT_TEMPLATE = "default";
    private static final String DEFAULT_USE_ALBUM_IMAGE_FILE = "0";
    private static final String DEFAULT_SHOW_ALBUM_IMAGES = "1";
    private static final String DEFAULT_USE_SOUND_SERVER = "1";
    private static final String DEFAULT_ADMIN_PASSWORD = "";
    private static final String DEFAULT_DEMO_MODE = "0";
    
    /**
     *
     * Loads options from options file, exit66jb.opt, or sets defaults
     *
     */
    public static void loadOptions() {
        
        options = new Properties();
        
        try {
            
            FileInputStream in = new FileInputStream(getOptionsFileName());
            options.load(in);
            
        } catch (FileNotFoundException e) {} catch (Exception e) {
            System.err.println(e);
        }
        
        if (options.getProperty("webserverport") == null)
            options.setProperty("webserverport", DEFAULT_WEBSERVERPORT);
        
        if (options.getProperty("webdirectory") == null)
            options.setProperty("webdirectory", DEFAULT_WEBDIRECTORY);
        
        if (options.getProperty("cachedirectory") == null)
            options.setProperty("cachedirectory", DEFAULT_CACHEDIRECTORY);
        checkCachedDirectory();
        
        if (options.getProperty("datadirectory") == null)
            options.setProperty("datadirectory", getDatabaseName());
        
        if (options.getProperty("unknownartist") == null)
            options.setProperty("unknownartist", DEFAULT_UNKNOWNARTIST);
        
        if (options.getProperty("unknownalbum") == null)
            options.setProperty("unknownalbum", DEFAULT_UNKNOWNALBUM);
        
        if (options.getProperty("variousartist") == null)
            options.setProperty("variousartist", DEFAULT_VARIOUSARTIST);
        
        if (options.getProperty("defaultfile") == null)
            options.setProperty("defaultfile", DEFAULT_DEFAULTFILE);
        
        if (options.getProperty("randomcount") == null)
            options.setProperty("randomcount", DEFAULT_RANDOMCOUNT);
        
        if (options.getProperty("scanthreadcount") == null)
            options.setProperty("scanthreadcount", DEFAULT_SCANTHREADCOUNT);
        
        if (options.getProperty("ignorealpha") == null)
            options.setProperty("ignorealpha", DEFAULT_IGNOREALPHA);

        if (options.getProperty("replace") == null)
            options.setProperty("replace", DEFAULT_REPLACE);
        _replace = null;
        
        if (options.getProperty("historycount") == null)
            options.setProperty("historycount", DEFAULT_HISTORYCOUNT);
        
        if (options.getProperty("playlisthistorycount") == null)
            options.setProperty("playlisthistorycount", DEFAULT_PLAYLISTHISTORYCOUNT);
        
        if (options.getProperty("defaultimagefile") == null)
            options.setProperty("defaultimagefile", DEFAULT_DEFAULTIMAGE);
        
        if (options.getProperty("databaseversion") == null)
            options.setProperty("databaseversion", DEFAULT_DATABASEVERSION);
        
        if (options.getProperty("defaulttemplate") == null)
            options.setProperty("defaulttemplate", DEFAULT_TEMPLATE);
        
        if (options.getProperty("usealbumimagefile") == null)
            options.setProperty("usealbumimagefile", DEFAULT_USE_ALBUM_IMAGE_FILE);
         
        if (options.getProperty("showalbumimages") == null)
            options.setProperty("showalbumimages", DEFAULT_SHOW_ALBUM_IMAGES);
        
        if (options.getProperty("usesoundserver") == null)
            options.setProperty("usesoundserver", DEFAULT_USE_SOUND_SERVER);
        
        if (options.getProperty("demomode") == null)
            options.setProperty("demomode", DEFAULT_DEMO_MODE);
        
        if (options.getProperty("adminpassword") == null)
            try {
                setAdminPassword(DEFAULT_ADMIN_PASSWORD);
            }
            catch (Exception e) {
                System.err.println(e.getMessage());
            }
        
        saveOptions();
        
    }
    
    private static boolean useMachineName() {
    	File f = new File(System.getProperty("user.dir") + System.getProperty("file.separator") + "usehostname");
    	return f.exists();
    }
    
    private static void checkCachedDirectory() {
    	if (!new File(options.getProperty("cachedirectory")).exists()) {
    		new File(options.getProperty("cachedirectory")).mkdirs();
    	}
    }
    
    private static String getOptionsFileName() {
    	try {
    		if (useMachineName()) {
    			InetAddress i = InetAddress.getLocalHost();
    			return "exit66jb-" + i.getHostName() + ".opt";
    		}
    		else {
    			return "exit66jb.opt";
    		}
    	}
    	catch (Exception e) {
    		return "exit66jb.opt";
    	}
    }
    
    private static String getDatabaseName() {
    	
    	
    	try {
    		if (useMachineName()) {
    			InetAddress i = InetAddress.getLocalHost();
    			return System.getProperty("user.dir") + System.getProperty("file.separator") + "data-" + i.getHostName() + System.getProperty("file.separator");
    		}
    		else {
    			return System.getProperty("user.dir") + System.getProperty("file.separator") + "data" + System.getProperty("file.separator");
    		}
    	}
    	catch (Exception e) {
    		return System.getProperty("user.dir") + System.getProperty("file.separator") + "data" + System.getProperty("file.separator");
    	}
    }
    
    /**
     *
     * Saves options to options file, exit66jb.opt.
     *
     */
    public static void saveOptions() {
        
        try {
            
            FileOutputStream out = new FileOutputStream(getOptionsFileName());
            options.store(out, "Exit66 Jukebox Options");
            
        } catch (Exception e) {
            
            System.err.println(e);
            
        }
        
    }
    
    /**
     *
     * Returns port the web server is on.
     *
     */
    public static int getWebServerPort() {
        
        return Integer.parseInt(options.getProperty("webserverport", DEFAULT_WEBSERVERPORT));
        
    }

    /**
     *
     * Returns the home directory for the web server.
     *
     */
    public static String getWebDirectory() {
        
        return options.getProperty("webdirectory", DEFAULT_WEBDIRECTORY);
        
    }


   public static boolean inDemoMode() {
       
       return options.getProperty("demomode", DEFAULT_DEMO_MODE).equals("1");
       
   }
    
    public static String getCacheDirectory() {
        
        return options.getProperty("cachedirectory", DEFAULT_CACHEDIRECTORY);
        
    }
    
    
    /**
     *
     * Returns the jukeBox database version.
     *
     */
    public static int getDatabaseVersion() {
        
        return Integer.parseInt(options.getProperty("databaseversion", DEFAULT_DATABASEVERSION));
        
    }
    
    /**
     *
     * Returns the home directory for the database server.
     *
     */
    public static String getDBDirectory() {
        
        return options.getProperty("datadirectory", getDatabaseName());
        
    }
    
    /**
     *
     * Returns the text to use for an unknown artist.
     *
     */
    public static String getUnknownArtist() {
        
        return options.getProperty("unknownartist", DEFAULT_UNKNOWNARTIST);
        
    }
    
    /**
     *
     * Returns the text to use for an unknown album.
     *
     */
    public static String getUnknownAlbum() {
        
        return options.getProperty("unknownalbum", DEFAULT_UNKNOWNALBUM);
        
    }
    
    /**
     *
     * Returns the text to use for albums with multiple artists.
     *
     */
    public static String getVariousArtist() {
        
        return options.getProperty("variousartist", DEFAULT_VARIOUSARTIST);
        
    }
    
    /**
     *
     * Returns the default file name to use for webbrowsing.  More specifically,
     * the file to look for if none is specified in the URL.
     *
     */
    public static String getDefaultFile() {
        
        return options.getProperty("defaultfile", DEFAULT_DEFAULTFILE);
        
    }
    
    /**
     *
     * Returns the whether to show the album covers or not
     *
     */
    public static int getShowAlbumImages() {
        
        return Integer.parseInt(options.getProperty("showalbumimages", DEFAULT_SHOW_ALBUM_IMAGES));
        
    }
    
    public static int getUseSoundServer() {
        
        return Integer.parseInt(options.getProperty("usesoundserver", DEFAULT_USE_SOUND_SERVER));
        
    }
    
    /**
     *
     * Returns Properties containing mime types and their file extensions.
     *
     */
    public static Properties getMimeTypes() {
        
        Properties mimeTypes = new Properties();
        
        String mimeFile = System.getProperty("user.dir");
        String line;
        
        if (mimeFile.substring(mimeFile.length() - 1).compareTo(System.getProperty("file.separator")) != 0) {
            
            mimeFile = mimeFile.concat(System.getProperty("file.separator"));
            
        }
        mimeFile = mimeFile.concat("mime.types");
        
        File f = new File(mimeFile);
        
        try {
            
            if (f.canRead()) {
                
                FileInputStream fis = new FileInputStream(f);
                BufferedReader fbr = new BufferedReader(new InputStreamReader(fis));
                
                line = fbr.readLine();
                while (line != null) {
                    
                    line = line.trim();
                    if ((line.length() > 0) && (line.charAt(0) != '#')) {
                        
                        int next = -1;
                        
                        if ((line.indexOf(' ') != -1) && (line.indexOf('\t') != -1)) {
                            
                            next = Math.min(line.indexOf(' '), line.indexOf('\t'));
                            
                        } else if (line.indexOf(' ') != -1) {
                            
                            next = line.indexOf(' ');
                            
                        } else {
                            
                            next = line.indexOf('\t');
                            
                        }
                        
                        if (next != -1){
                            
                            String mimeType = line.substring(0, next).trim();
                            line = line.substring(next).trim();
                            String[] ext = splitString(line, " ");
                            
                            for (int i=0; i<ext.length; i++) {
                                
                                if (ext[i].trim().length() > 0) {
                                    
                                    mimeTypes.put(ext[i], mimeType);
                                    
                                }
                                
                            }
                            
                        }
                        
                    }
                    line = fbr.readLine();
                    
                }
                
            }
            
        } catch (FileNotFoundException e) {
            System.err.println(e);
        } catch (IOException e) {
            System.err.println(e);
        }
        
        return mimeTypes;
        
    }
    
    /**
     *
     * Splits a string into an array based on the token passed.
     *
     * @param	input	the string to break apart
     * @param	spe	the token to break the string based on
     *
     */
    public static String[] splitString(String input, String sep) {
        
        StringTokenizer st = new StringTokenizer(input, sep);
        String[] t = new String[st.countTokens()];
        while (st.hasMoreTokens()) t[t.length-st.countTokens()] = st.nextToken();
        
        return t;
        
    }
    
    /**
     *
     * Returns the number of files to randomly choose from in the queue
     *
     */
    public static int getRandomCount() {
        
        return Integer.parseInt(options.getProperty("randomcount", DEFAULT_RANDOMCOUNT));
        
    }
    
    /**
     *
     * Returns the default string to use for unknown music type.
     *
     */
    public static String getUnknownContent() {
        
        return options.getProperty("unknowncontent", DEFAULT_UNKNOWNCONTENT);
        
    }
    
    /**
     *
     * Returns the number of threads to use while scanning for files.
     *
     */
    public static int getScanThreadCount() {
        
        return Integer.parseInt(options.getProperty("scanthreadcount", DEFAULT_SCANTHREADCOUNT));
        
    }
    
    /**
     *
     * Returns an array of strings to ignore for alphabetical purposes when they are at the beginning
     * of the artist's name.
     *
     */
    public static String[] getIgnoreAlpha() {
        
        String[] words = splitString(options.getProperty("ingorealpha", DEFAULT_IGNOREALPHA), ",");
        return words;
        
    }
    
    /**
    *
    * Returns a map of strings to replace with other strings for purposes of matching the artist's name
    *
    */
    public static Map<String, String> getReplace() {
        
    	if (_replace != null) {
    		return _replace;
    	}
    	
    	Map<String, String> replacemap = new HashMap<String, String>();
    	String[] replacetemp = splitString(options.getProperty("replace", DEFAULT_REPLACE), ",");
    	
    	for (int i=0; i<replacetemp.length; i++) {
    		String[] kv = replacetemp[i].split(":");
    		if (kv.length == 2) {
    			if (!replacemap.containsKey(kv[0])) {
    				replacemap.put(kv[0], kv[1]);
    			}
    		}
    	}
        
    	_replace = replacemap;
    	return replacemap;
    }
    
    /**
     *
     * Returns the number of songs in the history list to look through when attempting to play
     * another song.
     *
     */
    public static int getHistoryCount() {
        
        return Integer.parseInt(options.getProperty("historycount", DEFAULT_HISTORYCOUNT));
        
    }
    
    /**
     *
     * Returns the number of files to randomly choose from in the playlist
     *
     */
    public static int getPlaylistHistoryCount() {
        
        return Integer.parseInt(options.getProperty("playlisthistorycount", DEFAULT_PLAYLISTHISTORYCOUNT));
        
    }
    
    /**
     *
     * Sets the port for the web server
     *
     * @param	newValue	the new port number
     *
     */
    public static void setWebserverPort(int newValue) {
        
        options.setProperty("webserverport", String.valueOf(newValue));
        saveOptions();
        
    }
    
    /**
     *
     * Sets the jukeBox database version
     *
     * @param	newValue	the new database version
     *
     */
    public static void setDatabaseVersion(int newValue) {
        
        options.setProperty("databaseversion", String.valueOf(newValue));
        saveOptions();
        
    }
    
    public static void setDefaultFile(String newValue) {
        
        options.setProperty("defaultfile", newValue);
        saveOptions();
        
    }
    
    public static String getAdminPassword() {
        return options.getProperty("adminpassword");
    }
    
    public static void setAdminPassword(String newValue) {
        try {
            if (newValue.equals("")) {
                options.setProperty("adminpassword", "");
            }
            else {
                options.setProperty("adminpassword", new String(encrypt(newValue)));
            }
            saveOptions();
        }
        catch (Exception e) {
            System.err.println(e.getMessage());
        }
    }
    
    public static boolean verifyAdminPassword(String password) {
        try {
            if (password.equals("") || Options.getAdminPassword().equals("")) {
                return true;
            }
            String decoded = new String(Options.encrypt(password));
            return (decoded.compareTo(Options.getAdminPassword()) == 0);
        }
        catch (Exception e) {
            System.err.println(e.getMessage());
            return false;
        }
    }
    
    /**
     *
     * Returns the file name to use for logging.
     *
     */
    public static String nextLogFile() {
        
        try {
            
            File log = new File("exit66jb.log");
            log.delete();
            
        } catch (Exception e) {}
        return "exit66jb.log";
        
    }
    
    /**
     *
     * Returns the name of the image file to use when no cover image file is in
     * the audio file
     *
     */
    public static String getDefaultImageFileName() {
        
        return options.getProperty("defaultimagefile", DEFAULT_DEFAULTIMAGE);
        
    }
    
    /**
     *
     * Returns the mime type of the default image file to use when no cover image
     * file is in the audio file
     *
     */
    public static String getDefaultImageMimeType() {
        
        return "image/" + getExtension(Options.getDefaultImageFileName()).replace("jpg", "jpeg");
        
    }
    
    /**
     *
     * Writes the default cover image file out.
     *
     * @param	os	the DataOutputStream to write the image to.
     *
     */
    public static void outputDefaultImage(DataOutputStream os) {
        
        try {
            
            byte outByte[] = new byte[1024];
            FileInputStream fis = new FileInputStream(new File(Options.getDefaultImageFileName()));
            
            int j;
            int size = fis.available();
            
            for (j=0; j < size; ) {
                
                if (1024 > (size - j)) {
                    
                    fis.read(outByte, 0, (size - j - 1));
                } else {
                    
                    fis.read( outByte, 0, 1024);
                    
                }
                
                os.write(outByte);
                
                j = j + 1024;
                
            }
            fis.close();
            
        } catch (Exception e) {
            
            System.err.println(e);
            
        }
        
    }
    
    /**
     *
     * Returns the extension of a file
     *
     * @param	fileName	the name of the file to get the extension from
     *
     */
    public static String getExtension(String fileName) {
        
        int dot = fileName.lastIndexOf(".");
        
        if	(dot >= 0) {
            
            return fileName.substring(dot+1).toLowerCase();
            
        } else {
            
            return "";
            
        }
        
    }
    
    /**
     *
     * Returns the current date and time in the format yyyy/mm/dd hh:mm:ss
     *
     */
    public static String getCurrentTime() {
        
        Calendar cal = new GregorianCalendar();
        
        return cal.get(Calendar.YEAR) + "/" + cal.get(Calendar.MONTH) + "/" + cal.get(Calendar.DATE) + " " +
                cal.get(Calendar.HOUR) + ":" + cal.get(Calendar.MINUTE) + ":" + cal.get(Calendar.SECOND);
        
    }
    
    /**
     *
     * Returns the default template to use when none specified.
     *
     */
    public static String getDefaultTemplate() {
        
        return options.getProperty("defaulttemplate", DEFAULT_TEMPLATE);
        
    }
    
    /**
     *
     * Returns the value of whether a album image should be searched for in
     * the song's directory if none is found in the ID3 tag
     *
     */
    public static String getUseAlbumImageFile() {
        
        return options.getProperty("usealbumimagefile",
                DEFAULT_USE_ALBUM_IMAGE_FILE);
        
    }
            
    public static byte[] encrypt(String x) throws Exception {
        java.security.MessageDigest d =null;
        d = java.security.MessageDigest.getInstance("SHA-1");
        d.reset();
        d.update(x.getBytes());
        return  d.digest();
    }
    
    
    public static String getCurrentUTCTime() {
        TimeZone tz = TimeZone.getTimeZone("GMT:00");

        DateFormat dfGMT = DateFormat.getDateTimeInstance( DateFormat.LONG, DateFormat.LONG );
        dfGMT.setTimeZone( tz );

        return dfGMT.format( new Date() );
    }
    
    public static String getFileTime(long time) {
        TimeZone tz = TimeZone.getTimeZone("GMT:00");

        DateFormat dfGMT = DateFormat.getDateTimeInstance( DateFormat.LONG, DateFormat.LONG );
        dfGMT.setTimeZone( tz );

        return dfGMT.format( new Date(time) );
    }

	public static boolean isInteger(String possibleInt) {
		for (int i = 0; i < possibleInt.length(); i++) {
			char c = possibleInt.charAt(i);
			if (!((Character.isDigit(c)) || (i == 0 && c == '-')))
				return false;
		}
		return true;
	}
}
