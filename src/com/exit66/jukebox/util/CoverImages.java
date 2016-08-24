package com.exit66.jukebox.util;

import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.Transparency;
import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.FilenameFilter;
import java.io.IOException;
import java.io.InputStream;

import javax.imageio.ImageIO;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.servlet.ServletOutputStream;

import com.exit66.jukebox.Options;
import com.exit66.jukebox.data.Album;
import com.exit66.jukebox.data.AlbumCollection;
import com.exit66.jukebox.data.Track;
import com.exit66.jukebox.tag.TagReader;
import com.exit66.jukebox.tag.TagReaderFactory;

public class CoverImages {

	int _width = 0;
	int _height = 0;

	public void getArtistImage(HttpServletRequest req,
			HttpServletResponse resp, int artistID) throws IOException {
		AlbumCollection list = new AlbumCollection();
		list.listByArtist(artistID, 0, 1);
		getAlbumImage(req, resp, ((Album) list.getCurrent()).getAlbumID());
	}

	public void getTrackImage(HttpServletRequest req, HttpServletResponse resp,
			int trackID) throws IOException {
		Track track = new Track();
		track.setTrackID(trackID);
		if (track.fetch()) {
			getAlbumImage(req, resp, track.getAlbumID());
		}
	}

	public void getAlbumImage(HttpServletRequest req, HttpServletResponse resp,
			int albumId) throws IOException {
		ServletOutputStream out;

		if (req.getParameter("size") != null) {
			String[] size = req.getParameter("size").split(",");
			_width = Integer.parseInt(size[0]);
			if (size.length == 1) {
				_height = _width;
			} else {
				_height = Integer.parseInt(size[1]);
			}
		}

		out = resp.getOutputStream();

		Album album = new Album();
		album.setAlbumID(albumId);
		if (!album.fetch()) {
			sendBlankImage(resp, out);
			out.close();
			return;
		}

		if (sendCachedImage(album, resp, out)) {
			out.close();
			return;
		}

		if (sendTagImage(album, resp, out)) {
			out.close();
			return;
		}

		if (sendFolderImage(album, resp, out)) {
			out.close();
			return;
		}

		sendBlankImage(album, resp, out);
		out.close();
	}

	protected boolean sendCachedImage(Album album, HttpServletResponse res,
			ServletOutputStream out) {
		String cachedName;
		try {
			cachedName = getCachedFileName(album);
			if (cachedName == null)
				return false;

			File cachedFile = new File(cachedName);
			File cachedFileType = new File(cachedName + "-t");
			if (cachedFile.exists()) {
				if (cachedFileType.exists()) {
					DataInputStream ds = new DataInputStream(
							new FileInputStream(cachedFileType));
					res.setContentType(ds.readUTF());
					ds.close();
				} else {
					res.setContentType("image/jpeg");
				}
				writeFile(cachedFile, out);
				return true;
			}

		} catch (Exception e) {
			System.err.println(e);
			return false;
		}

		return false;
	}

	protected String getCachedFileName(Album album) {
		try {
			MD5 md5 = new MD5(album.getAlbumName());
			return Options.getCacheDirectory() + album.getAlbumID() + "-"
					+ _width + "-" + _height + md5.compute() + "-cache.jpg";
		} catch (Exception e) {
			return null;
		}
	}

	protected boolean sendTagImage(Album album, HttpServletResponse res,
			ServletOutputStream out) {
		try {
			Track track = album.getFirstTrackForAlbum();
			if (track == null)
				return false;
			TagReader tag = TagReaderFactory.getTagReader(track
					.getFullFileName());
			if ((tag.loadCoverImage())
					&& (tag.getCoverMimeType().length() != 0)) {
				res.setContentType(tag.getCoverMimeType());

				String cacheFileName = getCachedFileName(album);
				File cacheFile = new File(cacheFileName);
				FileOutputStream cacheFs = new FileOutputStream(cacheFile);

				writeCachedImageFileType(cacheFileName, tag.getCoverMimeType());
				scaleImage(new ByteArrayInputStream(tag.getCoverImage()),
						res.getContentType(), out, cacheFs);
				cacheFs.close();
				return true;
			}
		} catch (Exception e) {
			System.err.println(e);
		}
		return false;
	}

	protected boolean sendFolderImage(Album album, HttpServletResponse res,
			ServletOutputStream out) {
		try {

			Track track = album.getFirstTrackForAlbum();
			if (track == null)
				return false;
			File file = new File(track.getFullFileName());
			ImageFilter filter = new ImageFilter();

			File directory = file.getParentFile();
			String[] images = directory.list(filter);

			if ((images != null) && (images.length >= 1)) {
				String upperCaseName = images[0].toUpperCase();
				String contentType = "";
				if (upperCaseName.endsWith(".JPG")
						|| upperCaseName.endsWith(".JPEG")) {
					contentType = "image/jpeg";
				} else if (upperCaseName.endsWith(".PNG")) {
					contentType = "image/png";
				} else if (upperCaseName.endsWith(".GIF")) {
					contentType = "image/gif";
				}
				res.setContentType(contentType);
				writeFile(new File(directory.getPath(), images[0]), out, album,
						contentType);
				return true;
			}

		} catch (Exception e) {
			System.err.println(e);
		}
		return false;

	}

	protected void sendBlankImage(HttpServletResponse res,
			ServletOutputStream out) {
		sendBlankImage(null, res, out);
	}

	protected void sendBlankImage(Album album, HttpServletResponse res,
			ServletOutputStream out) {
		File blankImage = new File(Options.getDefaultImageFileName());
		if (blankImage.exists()) {
			res.setContentType(Options.getDefaultImageMimeType());
			if (album != null) {
				writeFile(blankImage, out, album,
						Options.getDefaultImageMimeType());
			} else {
				writeFile(blankImage, out);
			}
		}
	}

	protected void writeCachedImage(Album album, byte[] data, String mimeType) {
		String cacheFileName = getCachedFileName(album);
		if (cacheFileName != null) {
			writeOutputToFile(cacheFileName, data);
			writeCachedImageFileType(cacheFileName, mimeType);
		}
	}

	protected void writeCachedImageFileType(String cacheFileName,
			String mimeType) {
		File file = new File(cacheFileName + "-t");
		try {
			DataOutputStream ds = new DataOutputStream(new FileOutputStream(
					file));
			ds.writeUTF(mimeType);
			ds.close();
		} catch (Exception e) {
			System.err.println(e);
		}
	}

	protected void writeFile(File file, ServletOutputStream out) {
		writeFile(file, out, null, null);
	}

	protected void writeFile(File file, ServletOutputStream out,
			Album cacheForAlbum, String mimeType) {

		try {
			File cacheFile = null;
			FileOutputStream cacheFs = null;
			String cacheFileName;
			if (cacheForAlbum != null) {
				cacheFileName = getCachedFileName(cacheForAlbum);
				cacheFile = new File(cacheFileName);
				cacheFs = new FileOutputStream(cacheFile);
				writeCachedImageFileType(cacheFileName, mimeType);
			}
			scaleImage(new FileInputStream(file), mimeType, out, cacheFs);
			if (cacheFile != null)
				cacheFs.close();
		} catch (FileNotFoundException e) {
			System.err.println(e);
		} catch (IOException e) {
			System.err.println(e);
		}

	}

	protected void writeOutputToFile(String fileName, byte[] out) {
		try {
			File file = new File(fileName);
			FileOutputStream fs = new FileOutputStream(file);
			fs.write(out);
			fs.close();
		} catch (IOException e) {
			System.err.println(e);
		}
	}

	private void adjustRatio(BufferedImage origImage) {
		// adjust the dest width or height to maintain the aspect ratio
		double origRatio = (double) origImage.getWidth()
				/ (double) origImage.getHeight();
		double destRatio = (double) _width / (double) _height;
		if (origRatio != destRatio) {
			if (destRatio > 1) {
				// width is greater than height
				_width = (int) Math.round(_height * origRatio);
			} else {
				// width is less than the height
				_height = (int) Math.round(_width / origRatio);
			}
		}
	}

	private void scaleImage(InputStream input, String mimeType,
			ServletOutputStream out, FileOutputStream cacheFs) {
		if ((_width == 0) || (_height == 0) || (mimeType == null)) {
			try {
				byte[] tmp = new byte[1024];
				int bytes;
				while ((bytes = input.read(tmp)) != -1) {
					out.write(tmp, 0, bytes);
					if (cacheFs != null)
						cacheFs.write(tmp);
				}
			} catch (IOException e) {
				e.printStackTrace();
			}
		} else {
			try {
				String formatName = "jpg";

				if (mimeType.toLowerCase().endsWith("png")) {
					formatName = "png";
				} else if (mimeType.toLowerCase().endsWith("gif")) {
					formatName = "gif";
				}

				BufferedImage origImage = ImageIO.read(input);

				adjustRatio(origImage);
				
				BufferedImage destImage = getScaledInstance(origImage, _width, _height, RenderingHints.VALUE_INTERPOLATION_BILINEAR, true);

				ImageIO.write(destImage, formatName, out);
				if (cacheFs != null) {
					ImageIO.write(destImage, formatName, cacheFs);
				}
			} catch (Exception e) {
				e.printStackTrace();
			}
		}
	}

	public BufferedImage getScaledInstance(BufferedImage img, int targetWidth,
			int targetHeight, Object hint, boolean higherQuality) {
		int type = (img.getTransparency() == Transparency.OPAQUE) ? BufferedImage.TYPE_INT_RGB
				: BufferedImage.TYPE_INT_ARGB;
		BufferedImage ret = (BufferedImage) img;
		int w, h;
		if (higherQuality) {
			// Use multi-step technique: start with original size, then
			// scale down in multiple passes with drawImage()
			// until the target size is reached
			w = img.getWidth();
			h = img.getHeight();
		} else {
			// Use one-step technique: scale directly from original
			// size to target size with a single drawImage() call
			w = targetWidth;
			h = targetHeight;
		}

		do {
			if (higherQuality && w > targetWidth) {
				w /= 2;
				if (w < targetWidth) {
					w = targetWidth;
				}
			} else {
				w = targetWidth;
			}

			if (higherQuality && h > targetHeight) {
				h /= 2;
				if (h < targetHeight) {
					h = targetHeight;
				}
			} else {
				h = targetHeight;
			}

			BufferedImage tmp = new BufferedImage(w, h, type);
			Graphics2D g2 = tmp.createGraphics();
			g2.setRenderingHint(RenderingHints.KEY_INTERPOLATION, hint);
			g2.drawImage(ret, 0, 0, w, h, null);
			g2.dispose();

			ret = tmp;
		} while (w != targetWidth || h != targetHeight);

		return ret;
	}
}

class ImageFilter implements FilenameFilter {
	public boolean accept(File dir, String name) {
		String upperCaseName = name.toUpperCase();
		return (upperCaseName.endsWith(".JPG")
				|| upperCaseName.endsWith(".JPEG")
				|| upperCaseName.endsWith(".PNG") || upperCaseName
				.endsWith(".GIF"));
	}
}
