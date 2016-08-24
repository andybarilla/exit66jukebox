package com.exit66.jukebox.tag;

import java.io.*;
import java.util.Vector;

import org.cmc.music.metadata.IMusicMetadata;
import org.cmc.music.metadata.ImageData;
import org.cmc.music.metadata.MusicMetadataSet;
import org.cmc.music.myid3.*;

/**
 * This class gives information (audio format and comments) about MPEG file.
 */
public class ID3Reader extends TagReader
{
	   private static final String[] id3v1Tags = {
               "Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge", "Hip-Hop", "Jazz", "Metal",
               "New Age", "Oldies", "Other", "Pop", "R&B", "Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative",
               "Ska", "Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient", "Trip-Hop", "Vocal", "Jazz+Funk",
               "Fusion", "Trance", "Classical", "Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise",
               "AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative", "Instrum. Pop", "Instrum. Rock", "Ethnic",
               "Gothic", "Darkwave", "Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream", "Southern Rock",
               "Comedy", "Cult", "Gangsta", "Top", "Christian Rap", "Pop/Funk", "Jungle", "Native American", "Cabaret",
               "New Wave", "Psychadelic", "Rave", "Showtunes", "Trailer", "Lo-Fi", "Tribal", "Acid Punk", "Acid Jazz",
               "Polka", "Retro", "Musical", "Rock & Roll", "Hard Rock", "Folk", "Folk-Rock", "National Folk", "Swing",
               "Fast Fusion", "Bebob", "Latin", "Revival", "Celtic", "Bluegrass", "Avantgarde", "Gothic Rock",
               "Prog. Rock", "Psychedel. Rock", "Symph. Rock", "Slow Rock", "Big Band", "Chorus", "Easy Listening",
               "Acoustic", "Humour", "Speech", "Chanson", "Opera", "Chamber Music", "Sonata", "Symphony", "Booty Bass",
               "Primus", "Porn Groove", "Satire", "Slow Jam", "Club", "Tango", "Samba", "Folklore", "Ballad", "Power Ballad",
               "Rhythmic Soul", "Freestyle", "Duet", "Punk Rock", "Drum Solo", "Acapella", "Euro-House", "Dance Hall"};
	   
	public ID3Reader(String fileName) {
		super(fileName);
	}

	public boolean readTag() {
		try {
			File file = new File(_fileName);
			if (!file.exists()) 
				return false;
			MusicMetadataSet src_set = new MyID3().read(file);
			if (src_set != null) {
				IMusicMetadata metadata = null;
				if (src_set.id3v2Raw != null) {
					metadata = src_set.id3v2Raw.values;
				} else {
					metadata = src_set.id3v1Raw.values;
				}
				_artistName = metadata.getArtist();
				_albumName = metadata.getAlbum();
				_albumArtistName = metadata.getBand();
				_trackName = metadata.getSongTitle();
				_trackNumber = toInt(metadata.getTrackNumberNumeric());
				_contentType = metadata.getGenreName();
				setTime(toInt(metadata.getDurationSeconds()));
				if ((_contentType.length() >= 3) && (_contentType.charAt(0) == '(') && (_contentType.charAt(_contentType.length() - 1) == ')')) {
					try {
						int id = Integer.parseInt(_contentType.substring(1, _contentType.length() - 1));
						if (id < id3v1Tags.length) {
							_contentType = id3v1Tags[id];
						}
					}
					catch (Exception e) {
						System.err.println(e);
					}
				}
				return true;
			}
		}
		catch (Exception e) {
			System.err.println(e);
		}
		return false;
	}
	
	public boolean loadCoverImage() {
		_coverImage = new ByteArrayOutputStream();
		
		try {
			File file = new File(_fileName);
			if (!file.exists()) 
				return false;
			MusicMetadataSet src_set = new MyID3().read(file);
			
			if ((src_set != null) && (src_set.id3v2Clean != null)) {
				Vector pics = src_set.id3v2Clean.getPictures();
				ImageData first_pic = (ImageData)pics.get(0);
				                
                _coverMimeType = first_pic.mimeType;
                _coverImage.write(first_pic.imageData, 0, first_pic.imageData.length);
                
				return true;
			}
		}
		catch (Exception e) {
			System.err.println(e);
		}
		
		return false;

	}
	
}